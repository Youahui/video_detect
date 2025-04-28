package engine

import (
	"context"
	"fmt"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"go_client/config"
	"go_client/pkg/map_utils"
	"runtime/debug"
	"sort"
	"sync"
	"time"
)

func GenPushURL(preURL, streamKey string) string {
	return preURL + streamKey
}

type SessionManager struct {
	pushUrlInternalPre string // 推流使用前缀 ：如 rtmp://rtmp-server/live/stream
	pushUrlPublicPre   string // 播放展示用：如 rtmp://mydomain.com/live/stream
	ctx                context.Context
	cancel             context.CancelFunc
	logger             *zap.Logger
	cfg                *config.Config
	sessionPool        sync.Pool
	sessions           *map_utils.Map[string, *Session]
	closeCh            chan string
	healthyHeartbeat   int32
}

func NewSessionManager(ctx context.Context, canalFunc context.CancelFunc, logger *zap.Logger, cfg *config.Config, healthyHeartbeat int32, pushUrlInternalPre, pushUrlPublicPre string) *SessionManager {
	return &SessionManager{
		pushUrlInternalPre: pushUrlInternalPre,
		pushUrlPublicPre:   pushUrlPublicPre,
		ctx:                ctx,
		cancel:             canalFunc,
		logger:             logger,
		sessionPool: sync.Pool{
			New: func() interface{} {
				return new(Session)
			}},
		sessions: map_utils.New[string, *Session](),
		closeCh:  make(chan string, 128),
		//rwLock:   new(sync.RWMutex),
		cfg:              cfg,
		healthyHeartbeat: healthyHeartbeat,
	}
}

func (s *SessionManager) Run() {
	go s.closeChRecv()
	go s.checkHealthySession()
}

func (s *SessionManager) Close() {
	s.logger.Info("sessionManager close...")
	s.sessions.Range(func(key string, _session *Session) bool {
		_session.cancelFunc()
		s.sessions.Delete(key)
		_session.Reset()
		s.sessionPool.Put(_session)
		return true
	})
	s.cancel()
}

// closeChRecv 接收关闭session并处理
func (s *SessionManager) closeChRecv() {
	s.logger.Info("session manager closeChRecv running...")
	for {
		select {
		case <-s.ctx.Done():
			s.logger.Info("🛑 SessionManager 关闭session处理程序关闭")
			return
		case id := <-s.closeCh:
			_session, exists := s.sessions.Load(id)
			if !exists {
				continue
			}
			s.logger.Info(fmt.Sprintf(`📴 Stream session "%v" 关闭会话并清除`, id))
			_session.cancelFunc()
			s.sessions.Delete(id)

			// 重置_session 并放回池中
			_session.Reset()
			s.sessionPool.Put(_session)
		}

	}
}

// CheckHealthySession 检查会话健康
func (s *SessionManager) checkHealthySession() {
	s.logger.Info("session manager checkHealthySession running...")
	tick := time.Tick(time.Second * time.Duration(s.healthyHeartbeat))
	for {
		select {
		case <-s.ctx.Done():
			s.logger.Info("🛑 SessionManager 会话健康检查关闭")
			return
		case <-tick:
			s.sessions.Range(func(key string, _session *Session) bool {
				if !_session.runningStatus.Load() {
					// 避免重复 cancel 和清理
					if _session.handledClose.CompareAndSwap(false, true) {
						_session.cancelFunc()
						s.sessions.Delete(key)
						_session.Reset()
						s.sessionPool.Put(_session)

						s.logger.Info("🧹 Tick 清理非运行 Session", zap.String("id", _session.id))
					}
				}
				return true
			})
		}
	}
}

func (s *SessionManager) CreateSession(id, rtsp, aiURL string, options ...SetSessionOption) (desc SessionDesc, err error) {
	if _, exists := s.sessions.Load(id); exists {
		return desc, fmt.Errorf("session already started: %s", id)
	}

	ctx, cancel := context.WithCancel(s.ctx)

	session := s.sessionPool.Get().(*Session)
	session.id = id
	session.rtspURL = rtsp
	session.cancelFunc = cancel
	session.ctx = ctx
	session.logger = s.logger
	session.closeCh = s.closeCh

	session.streamKey = uuid.New().String()

	session.SetSessionWithOptions(options...)
	session.resultCache = &DetectionResultCache{
		RWMutex: sync.RWMutex{},
		Results: make([]DetectionResult, 0),
	}
	session.frameForDetection = make(chan []byte, 32)

	s.sessions.Store(id, session)

	pushURL := GenPushURL(s.pushUrlInternalPre, session.streamKey)
	if err := session.PrepareStream(pushURL); err != nil {
		s.sessions.Delete(id)
		return desc, fmt.Errorf("failed to prepare stream: %w", err)
	}

	s.logger.Info("🚀 Session started", zap.String("id", id), zap.String("rtsp", rtsp), zap.String("pushRTMPURL", pushURL))

	go func() {
		defer func() {
			if r := recover(); r != nil {
				s.logger.Error("panic recovered in Session.Run", zap.Any("error", r), zap.ByteString("stack", debug.Stack()))
			}
		}()
		session.Run(aiURL, s.cfg.Engine.UvicornSocket, s.cfg.Engine.SocketPath)
	}()

	desc = session.GetDesc(s.pushUrlPublicPre)

	return desc, nil
}

func (s *SessionManager) GetSessionDescList() []SessionDesc {
	descList := make([]SessionDesc, 0)
	s.sessions.Range(func(key string, _session *Session) bool {
		if !_session.runningStatus.Load() {
			return true
		}
		descList = append(descList, _session.GetDesc(s.pushUrlPublicPre))
		return true
	})

	sort.Slice(descList, func(i, j int) bool {
		return descList[i].ID < descList[j].ID
	})

	return descList
}

func (s *SessionManager) StopSessionRun(id string) error {
	if session, exists := s.sessions.Load(id); exists {
		session.cancelFunc()
		return nil
	}
	return fmt.Errorf("Session 不存在: %s", id)
}

func (s *SessionManager) StopSessionDetect(id string) error {
	if session, exists := s.sessions.Load(id); exists {
		session.detectStatus.Store(false)
		return nil
	}
	return nil
}

func (s *SessionManager) StartSessionDetect(id string) error {
	if session, exists := s.sessions.Load(id); exists {
		session.detectStatus.Store(true)
		return nil
	}
	return nil
}

func (s *SessionManager) RemoveSession(id string) {
	_session, exists := s.sessions.Load(id)
	if !exists {
		return
	}
	_session.runningStatus.Store(false)
	s.sessions.Delete(id)
	_session.cancelFunc()
	_session.Reset()
	s.sessionPool.Put(_session)
}

func (s *SessionManager) GetSessionDescByID(id string) (SessionDesc, bool) {
	_session, exists := s.sessions.Load(id)
	if !exists {
		return SessionDesc{}, false
	}

	return _session.GetDesc(s.pushUrlPublicPre), true
}
