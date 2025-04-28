package engine

import (
	"encoding/base64"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"go_client/config"
	"go_client/pkg/result"
	"go_client/pkg/status"
	"gocv.io/x/gocv"
	"image"
	"image/color"
	"io"
	"net/http"
)

type DetectHTTPService interface {
	CreateSession(c *gin.Context) error      // 创建识别会话
	GetAllSessionDesc(c *gin.Context) error  // 获取会话描述列表
	GetSessionDescByID(c *gin.Context) error // 根据ID获取会话描述
	StopDetect(c *gin.Context) error         // 暂停识别（仍保持推流）
	StartDetect(c *gin.Context) error        // 继续识别（仍保持推流）
	RemoveSession(c *gin.Context) error      // 删除会话并停止拉流推流

	DetectTest(c *gin.Context) error // 测试
}

func RegisterDetectHTTPService(eng *gin.Engine, srv DetectHTTPService) {
	eng.POST("/test", WrapHandler(srv.DetectTest))

	detect := eng.Group("/detect/session")
	{
		detect.POST("", WrapHandler(srv.CreateSession))
		detect.GET("/list", WrapHandler(srv.GetAllSessionDesc))

		action := detect.Group("/:sessionID")
		{
			action.GET("", WrapHandler(srv.GetSessionDescByID))
			action.PUT("/detect/stop", WrapHandler(srv.StopDetect))
			action.PUT("/detect/start", WrapHandler(srv.StartDetect))
			action.DELETE("", WrapHandler(srv.RemoveSession))
		}
	}
}

// DetectHTTPServiceV1 识别引擎http服务
type DetectHTTPServiceV1 struct {
	cfg     *config.Config
	manager *SessionManager
	logger  *zap.Logger
}

func NewDetectHTTPServiceV1(cfg *config.Config, manager *SessionManager, logger *zap.Logger) DetectHTTPService {
	return DetectHTTPServiceV1{cfg, manager, logger}
}

func (d DetectHTTPServiceV1) DetectTest(c *gin.Context) error {
	// 1. 解析上传的文件
	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请上传文件"})
		return nil
	}

	openedFile, err := file.Open()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "文件打开失败"})
		return nil
	}
	defer openedFile.Close()

	imgBytes, err := io.ReadAll(openedFile)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "读取文件失败"})
		return nil
	}

	// 2. 调用 detectObjects
	aiURL := d.cfg.Engine.DetectAIURL
	results, err := detectObjects(imgBytes, aiURL)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return nil
	}

	// 3. 转换为 gocv.Mat
	imgMat, err := gocv.IMDecode(imgBytes, gocv.IMReadColor)
	if err != nil || imgMat.Empty() {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "图像解码失败"})
		return nil
	}
	defer imgMat.Close()

	// 4. 画识别框
	for _, r := range results {
		rect := image.Rect(r.X1, r.Y1, r.X2, r.Y2)
		_ = gocv.Rectangle(&imgMat, rect, color.RGBA{0, 255, 0, 0}, 2)
		_ = gocv.PutText(&imgMat, r.Label, image.Pt(r.X1, r.Y1-10),
			gocv.FontHersheyPlain, 1.2, color.RGBA{255, 0, 0, 0}, 2)
	}

	// 5. 编码结果为 JPEG 并转为 base64
	resultImgBytes, err := gocv.IMEncode(gocv.JPEGFileExt, imgMat)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "图像编码失败"})
		return nil
	}

	base64Str := base64.StdEncoding.EncodeToString(resultImgBytes.GetBytes())

	// 6. 返回 JSON
	c.JSON(http.StatusOK, gin.H{
		"message":      "检测完成",
		"results":      results,
		"image_base64": "data:image/jpeg;base64," + base64Str,
	})
	return nil
}

func (d DetectHTTPServiceV1) CreateSession(c *gin.Context) error {
	var req CreateSessionReq
	err := Bind(&req, c.Request.Body)
	if err != nil {
		return err
	}
	desc, err := d.manager.CreateSession(req.ID, req.RtspURL, d.cfg.Engine.DetectAIURL,
		SetSessionVideoStreamConfig(req.Width, req.Height, req.Framerate))
	if err != nil {
		return status.Wrapper(http.StatusInternalServerError, err)
	}

	result.New[SessionDesc](http.StatusOK).
		Data(desc).
		Ok(c.Writer)
	return nil
}

func (d DetectHTTPServiceV1) GetAllSessionDesc(c *gin.Context) error {
	descList := d.manager.GetSessionDescList()
	result.New[[]SessionDesc](http.StatusOK).Data(descList).Ok(c.Writer)
	return nil
}

func (d DetectHTTPServiceV1) GetSessionDescByID(c *gin.Context) error {
	var action SessionAction
	if err := BindParams(&action, c.Params); err != nil {
		return err
	}
	desc, ok := d.manager.GetSessionDescByID(action.SessionID)

	result.New[GetSessionDescByIDResp](http.StatusOK).
		Data(GetSessionDescByIDResp{ok, desc}).
		Ok(c.Writer)
	return nil
}

func (d DetectHTTPServiceV1) StopDetect(c *gin.Context) error {
	var action SessionAction
	if err := BindParams(&action, c.Params); err != nil {
		return err
	}

	err := d.manager.StopSessionDetect(action.SessionID)
	if err != nil {
		return status.Wrapper(http.StatusInternalServerError, err)
	}

	result.New[gin.H](http.StatusOK).Data(gin.H{"ok": true}).Ok(c.Writer)
	return nil
}

func (d DetectHTTPServiceV1) StartDetect(c *gin.Context) error {
	var action SessionAction
	if err := BindParams(&action, c.Params); err != nil {
		return err
	}

	err := d.manager.StartSessionDetect(action.SessionID)
	if err != nil {
		return status.Wrapper(http.StatusInternalServerError, err)
	}
	result.New[gin.H](http.StatusOK).Data(gin.H{"ok": true}).Ok(c.Writer)
	return nil
}

func (d DetectHTTPServiceV1) RemoveSession(c *gin.Context) error {
	var action SessionAction
	if err := BindParams(&action, c.Params); err != nil {
		return err
	}

	d.manager.RemoveSession(action.SessionID)

	result.New[gin.H](http.StatusOK).Data(gin.H{"ok": true}).Ok(c.Writer)
	return nil
}
