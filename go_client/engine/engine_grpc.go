package engine

import (
	"context"
	"go_client/config"
	"go_client/pb"
)

type DetectGRPCServiceV1 struct {
	cfg     *config.Config
	manager *SessionManager
	pb.UnimplementedDetectServiceServer
}

func (d DetectGRPCServiceV1) CreateSession(_ context.Context, req *pb.CreateSessionReq) (*pb.SessionDesc, error) {
	desc, err := d.manager.CreateSession(
		req.Id,
		req.RtspURL,
		d.cfg.Engine.DetectAIURL,
		SetSessionVideoStreamConfig(
			int(req.Width),
			int(req.Height),
			int(req.Framerate),
		))
	if err != nil {
		return nil, err
	}

	return &pb.SessionDesc{
		Id:            desc.ID,
		StreamKey:     desc.StreamKey,
		PushUrlPublic: desc.PushUrlPublic,
		DetectStatus:  desc.DetectStatus,
	}, nil
}

func (d DetectGRPCServiceV1) GetAllSessionDesc(_ context.Context, _ *pb.Empty) (*pb.AllSessionDescResp, error) {

	descList := d.manager.GetSessionDescList()

	res := make([]*pb.SessionDesc, len(descList))
	for i := range descList {
		res[i] = &pb.SessionDesc{
			Id:            descList[i].ID,
			StreamKey:     descList[i].StreamKey,
			PushUrlPublic: descList[i].PushUrlPublic,
			DetectStatus:  descList[i].DetectStatus,
		}
	}
	return &pb.AllSessionDescResp{
		Sessions: res,
	}, nil
}

func (d DetectGRPCServiceV1) GetSessionDescByID(_ context.Context, req *pb.SessionIDReq) (*pb.GetSessionDescByIDResp, error) {
	desc, ok := d.manager.GetSessionDescByID(req.SessionID)

	return &pb.GetSessionDescByIDResp{
		Exists: ok,
		Session: &pb.SessionDesc{
			Id:            desc.ID,
			StreamKey:     desc.StreamKey,
			PushUrlPublic: desc.PushUrlPublic,
			DetectStatus:  desc.DetectStatus,
		},
	}, nil
}

func (d DetectGRPCServiceV1) StopDetect(_ context.Context, req *pb.SessionIDReq) (*pb.GenericResp, error) {

	err := d.manager.StopSessionDetect(req.SessionID)

	return &pb.GenericResp{Ok: true}, err
}

func (d DetectGRPCServiceV1) ContinueDetect(_ context.Context, req *pb.SessionIDReq) (*pb.GenericResp, error) {

	err := d.manager.StartSessionDetect(req.SessionID)

	return &pb.GenericResp{Ok: true}, err
}

func (d DetectGRPCServiceV1) RemoveSession(_ context.Context, req *pb.SessionIDReq) (*pb.GenericResp, error) {

	d.manager.RemoveSession(req.SessionID)

	return &pb.GenericResp{Ok: true}, nil
}
