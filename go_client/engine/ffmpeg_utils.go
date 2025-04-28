package engine

import (
	"bufio"
	"fmt"
	"go.uber.org/zap"
	"io"
	"os/exec"
)

func startFFmpegReader(rtsp string, width, height, fps int) (*exec.Cmd, io.Reader, error) {
	cmd := exec.Command("ffmpeg",
		"-rtsp_transport", "tcp",
		"-i", rtsp,
		"-an",
		"-f", "rawvideo",
		"-pix_fmt", "bgr24",
		"-s", fmt.Sprintf("%dx%d", width, height),
		"-r", fmt.Sprintf("%d", fps),
		"-")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, nil, err
	}
	return cmd, stdout, nil
}

// startFFmpegPusher 开启推流FFmpeg 推流
func startFFmpegPusher(width, height int, framerate float64, isDebug bool, rtmpURL string, _logger *zap.Logger) (*exec.Cmd, io.WriteCloser, error) {
	args := make([]string, 0)
	if isDebug {
		args = append(args, "-loglevel", "debug") // 加入详细日志
	} else {
		args = append(args, "-loglevel", "error") // 只输出错误
	}

	args = append(args, []string{
		"-y", "-f", "rawvideo",
		"-pixel_format", "bgr24",
		"-video_size", fmt.Sprintf("%dx%d", width, height),
		"-framerate", fmt.Sprintf("%.2f", framerate),
		"-i", "-",
		"-c:v", "libx264", "-pix_fmt", "yuv420p",
		"-f", "flv", rtmpURL,
	}...)

	cmd := exec.Command("ffmpeg",
		args...,
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

	// 根据 isDebug 决定日志处理
	if isDebug {
		go func() {
			scanner := bufio.NewScanner(stderr)
			for scanner.Scan() {
				_logger.Info("FFmpeg: " + scanner.Text())
			}
		}()
	} else {
		// 即使不打印日志，也需要消费 stderr，防止阻塞
		go io.Copy(io.Discard, stderr)
	}

	if err := cmd.Start(); err != nil {
		stdin.Close()
		return nil, nil, err
	}

	return cmd, stdin, nil
}
