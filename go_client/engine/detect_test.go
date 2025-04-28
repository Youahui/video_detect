package engine

import (
	"bufio"
	"fmt"
	"gocv.io/x/gocv"
	"image"
	"image/color"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

type Config struct {
	DefaultAIURL string
	RTMPURL      string
	MaxRetries   int
	Framerate    float64
}

var (
	running     int32
	currentRTSP atomic.Value
	currentAI   atomic.Value
	configv1    Config
	//bufPool     sync.Pool
	loggerV1 *Logger
)

type Logger struct {
	sync.Mutex
}

func (l *Logger) Info(msg string) {
	l.Lock()
	defer l.Unlock()
	fmt.Printf("ℹ️ %s %s\n", time.Now().Format(time.RFC3339), msg)
}

func (l *Logger) Error(msg string, err error) {
	l.Lock()
	defer l.Unlock()
	fmt.Printf("❌ %s %s: %v\n", time.Now().Format(time.RFC3339), msg, err)
}

func loadConfig() Config {
	cfg := Config{
		DefaultAIURL: "http://host.docker.internal:5000/detect",
		RTMPURL:      "rtmp://rtmp-server/live/stream",
		//RTMPURL:      "rtmp://host.docker.internal/live/stream",
		MaxRetries: 10,
		Framerate:  25.0,
	}
	if aiURL := os.Getenv("AI_URL"); aiURL != "" {
		cfg.DefaultAIURL = aiURL
	}
	if rtmpURL := os.Getenv("RTMP_URL"); rtmpURL != "" {
		cfg.RTMPURL = rtmpURL
	}
	return cfg
}

func handleSignals() {
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	<-sigs
	loggerV1.Info("收到终止信号")
	atomic.StoreInt32(&running, 0)
	os.Exit(0)
}

func startFFmpeg(width, height int, framerate float64) (*exec.Cmd, io.WriteCloser, error) {
	cmd := exec.Command("ffmpeg",
		"-loglevel", "debug", // 加入详细日志
		"-y", "-f", "rawvideo",
		"-pixel_format", "bgr24",
		"-video_size", fmt.Sprintf("%dx%d", width, height),
		"-framerate", fmt.Sprintf("%.2f", framerate),
		"-i", "-",
		"-c:v", "libx264", "-pix_fmt", "yuv420p",
		"-f", "flv", configv1.RTMPURL,
	)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, nil, err
	}

	// 👇 捕获 stderr 日志输出
	stderr, err := cmd.StderrPipe()
	if err != nil {
		stdin.Close()
		return nil, nil, err
	}

	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			loggerV1.Info("FFmpeg: " + scanner.Text())
		}
	}()

	if err := cmd.Start(); err != nil {
		stdin.Close()
		return nil, nil, err
	}

	return cmd, stdin, nil
}

func runStream(rtsp string) error {
	loggerV1.Info("启动识别流: " + rtsp)

	width, height := 1280, 720 // 可配置，或用 ffprobe 预解析
	framerate := int(configv1.Framerate)

	// 启动拉流 ffmpeg（拉 RTSP → pipe）
	cmd, stdout, err := startFFmpegReader(rtsp, width, height, framerate)
	if err != nil {
		return fmt.Errorf("FFmpeg 拉流失败: %w", err)
	}
	defer cmd.Process.Kill()

	// 启动推流 ffmpeg（写入处理后图像）
	ffmpegOut, ffmpegIn, err := startFFmpeg(width, height, float64(framerate))
	if err != nil {
		return fmt.Errorf("FFmpeg 推流失败: %w", err)
	}
	defer func() {
		ffmpegIn.Close()
		ffmpegOut.Wait()
	}()

	imgBuf := make([]byte, width*height*3)
	img := gocv.NewMat()
	defer img.Close()

	failCount := 0

	for atomic.LoadInt32(&running) == 1 {
		_, err := io.ReadFull(stdout, imgBuf)
		if err != nil {
			failCount++
			loggerV1.Info(fmt.Sprintf("⚠️ 第 %d 次读取帧失败: %v", failCount, err))
			if failCount >= configv1.MaxRetries {
				return fmt.Errorf("连续 %d 次读取帧失败", configv1.MaxRetries)
			}
			time.Sleep(200 * time.Millisecond)
			continue
		}
		failCount = 0

		img, _ = gocv.NewMatFromBytes(height, width, gocv.MatTypeCV8UC3, imgBuf)
		if img.Empty() {
			continue
		}

		buf, err := gocv.IMEncode(".jpg", img)
		if err != nil {
			loggerV1.Error("帧编码失败", err)
			continue
		}
		ai := currentAI.Load().(string)
		results, err := detectObjects(buf.GetBytes(), ai)
		buf.Close()
		if err != nil {
			loggerV1.Error("AI 识别失败", err)
			continue
		}
		for _, r := range results {
			rect := image.Rect(r.X1, r.Y1, r.X2, r.Y2)
			gocv.Rectangle(&img, rect, color.RGBA{0, 255, 0, 0}, 2)
			gocv.PutText(&img, r.Label, image.Pt(r.X1, r.Y1-10),
				gocv.FontHersheyPlain, 1.2, color.RGBA{255, 0, 0, 0}, 2)
		}

		if _, err := ffmpegIn.Write(img.ToBytes()); err != nil {
			// 先保存帧
			saveErr := gocv.IMWrite(fmt.Sprintf("ffmpeg_error_frame_%d.jpg", time.Now().Unix()), img)
			if !saveErr {
				loggerV1.Info("⚠️ 图像保存失败")
			} else {
				loggerV1.Info("✅ 异常帧已保存")
			}
			loggerV1.Error("FFmpeg 写入失败", err)
		}
	}
	loggerV1.Info("停止流")
	return nil
}

func controlHTTP() {
	http.HandleFunc("/start", func(w http.ResponseWriter, r *http.Request) {
		rtsp := r.URL.Query().Get("rtsp")
		ai := r.URL.Query().Get("ai")
		if !isValidRTSP(rtsp) {
			http.Error(w, "RTSP 地址无效", http.StatusBadRequest)
			return
		}
		if ai == "" {
			ai = configv1.DefaultAIURL
		}
		if _, err := url.ParseRequestURI(ai); err != nil {
			http.Error(w, "AI 地址无效", http.StatusBadRequest)
			return
		}
		currentRTSP.Store(rtsp)
		currentAI.Store(ai)
		if atomic.CompareAndSwapInt32(&running, 0, 1) {
			fmt.Fprintf(w, "✅ 识别已启动\nRTSP: %s\nAI: %s", rtsp, ai)
		} else {
			http.Error(w, "⚠️ 已在运行中", http.StatusConflict)
		}
	})
	http.HandleFunc("/stop", func(w http.ResponseWriter, r *http.Request) {
		if atomic.CompareAndSwapInt32(&running, 1, 0) {
			fmt.Fprint(w, "🛑 已停止识别")
		} else {
			fmt.Fprint(w, "ℹ️ 当前未运行")
		}
	})

	http.HandleFunc("/test", func(w http.ResponseWriter, r *http.Request) {
		img := gocv.IMRead("test.jpg", gocv.IMReadColor)
		if img.Empty() {
			http.Error(w, "无法读取 test.jpg", http.StatusInternalServerError)
			return
		}
		defer img.Close()

		buf, err := gocv.IMEncode(".jpg", img)
		if err != nil {
			http.Error(w, "图片编码失败", http.StatusInternalServerError)
			return
		}
		defer buf.Close()

		results, err := detectObjects(buf.GetBytes(), configv1.DefaultAIURL)
		if err != nil {
			http.Error(w, "AI 识别失败: "+err.Error(), http.StatusInternalServerError)
			return
		}

		// 画框
		for _, r := range results {
			rect := image.Rect(r.X1, r.Y1, r.X2, r.Y2)
			gocv.Rectangle(&img, rect, color.RGBA{0, 255, 0, 0}, 2)
			gocv.PutText(&img, r.Label, image.Pt(r.X1, r.Y1-10),
				gocv.FontHersheyPlain, 1.2, color.RGBA{255, 0, 0, 0}, 2)
		}

		outputPath := "result.jpg"
		ok := gocv.IMWrite(outputPath, img)
		if !ok {
			http.Error(w, "保存 result.jpg 失败", http.StatusInternalServerError)
			return
		}
		w.Write([]byte("✅ 测试完成，已保存 result.jpg"))
	})
	loggerV1.Info("控制接口监听 http://localhost:8080")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		loggerV1.Error("HTTP 启动失败", err)
		os.Exit(1)
	}
}

func isValidRTSP(rtsp string) bool {
	return strings.HasPrefix(rtsp, "rtsp://") || strings.HasPrefix(rtsp, "rtmp://")
}

func TestRun(t *testing.T) {
	//webcam, err := gocv.VideoCaptureFile("rtsp://192.168.176.1:8554/1")
	//if err != nil {
	//	log.Fatal(err)
	//}
	//defer webcam.Close()
	//
	//img := gocv.NewMat()
	//defer img.Close()
	//
	//if ok := webcam.Read(&img); !ok || img.Empty() {
	//	log.Fatal("读取失败")
	//}
	//gocv.IMWrite("frame.jpg", img)

	configv1 = loadConfig()
	loggerV1 = &Logger{}
	//bufPool = sync.Pool{New: func() interface{} { return new(bytes.Buffer) }}
	currentAI.Store(configv1.DefaultAIURL)
	go handleSignals()
	go controlHTTP()
	for {
		if atomic.LoadInt32(&running) == 1 {
			rtsp := currentRTSP.Load().(string)
			if err := runStream(rtsp); err != nil {
				loggerV1.Error("运行流失败", err)
			}
		}
		time.Sleep(time.Second)
	}
}
