package engine

import (
	"context"
	"errors"
	"fmt"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"go_client/config"
	"go_client/pb"
	"go_client/pkg/logger"
	"golang.org/x/net/http2"
	"google.golang.org/grpc"
	"net"
	"net/http"
	"os"
)

// DetectionEngine 识别Engine
type DetectionEngine struct {
	ctx    context.Context
	cancel context.CancelFunc

	cfg     *config.Config
	manager *SessionManager
	router  *gin.Engine
	logger  *zap.Logger
	srv     *http.Server
	peerSrv *grpc.Server

	httpService DetectHTTPService
	grpcService pb.DetectServiceServer
}

func NewDetectEngine(args []string) (engine *DetectionEngine, err error) {
	ctx, cancelFunc := context.WithCancel(context.Background())
	defer func() {
		if err != nil {
			cancelFunc()
		}
	}()

	// init config
	_config, err := config.BindConfig(args)
	if err != nil {
		return nil, err
	}

	// new zap logger
	_logger := logger.NewLogWithSplitting(logger.LogSplitting{
		FileName:   _config.Logger.LogPath,
		MaxSize:    _config.Logger.SplitMaxSize,
		MaxAge:     _config.Logger.MaxAge,
		MaxBackups: _config.Logger.MaxBackups,
		LocalTime:  _config.Logger.LocalTime,
		Compress:   _config.Logger.Compress,
	}, _config.Logger.LogLevel)

	// new session manager
	_ctx, _cancel := context.WithCancel(ctx)
	_manager := NewSessionManager(
		_ctx,
		_cancel,
		_logger,
		_config,
		_config.Engine.HealthyHeartbeat,
		_config.Engine.PushUrlInternalPre,
		_config.Engine.PushUrlPublicPre,
	)

	// ---- init gin engine ----
	router := gin.New()
	router.MaxMultipartMemory = 32 << 20 // 16 MB
	router.UseH2C = _config.Server.UseH2C
	router.Use(
		gin.CustomRecovery(RecoveryMiddleware(_logger)),
		LoggerMiddleware(_logger),
	)

	_httpService := NewDetectHTTPServiceV1(_config, _manager, _logger)
	RegisterDetectHTTPService(router, _httpService)

	// new engine
	engine = &DetectionEngine{
		ctx:         ctx,
		cancel:      cancelFunc,
		cfg:         _config,
		manager:     _manager,
		router:      router,
		logger:      _logger,
		peerSrv:     grpc.NewServer(),
		httpService: _httpService,
	}

	_grpcService := &DetectGRPCServiceV1{
		manager: _manager,
	}
	pb.RegisterDetectServiceServer(engine.peerSrv, _grpcService)
	engine.grpcService = _grpcService

	// ---- init http server ----
	engine.srv = &http.Server{
		Addr:    engine.cfg.Server.ListenHttpAddr,
		Handler: engine.router,
	}

	return engine, http2.ConfigureServer(engine.srv, nil)
}

func (e *DetectionEngine) Run(endCh chan os.Signal) {
	e.manager.Run()

	go func() {
		defer func() {
			endCh <- os.Interrupt
		}()

		err := e.srv.ListenAndServe()
		if errors.Is(err, http.ErrServerClosed) {
			e.logger.Warn(fmt.Sprintf("http server error: %v", err))
			return
		}
		return
	}()

	go func() {
		defer func() {
			endCh <- os.Interrupt
		}()

		listen, err := net.Listen("tcp", e.cfg.Server.GrpcPeerAddr)
		if err != nil {
			e.logger.Warn(fmt.Sprintf("grpc server listen error: %v", err))
			return
		}

		err = e.peerSrv.Serve(listen)
		if err != nil {
			e.logger.Warn(fmt.Sprintf("grpc server error: %v", err))
			return
		}
	}()

	return
}

func (e *DetectionEngine) Close() {
	defer func() {
		// DON'T CHECK RECOVER RETURN VALUE
		_ = recover()
	}()

	e.peerSrv.Stop()
	_ = e.srv.Shutdown(e.ctx)

	e.manager.Close()
}
