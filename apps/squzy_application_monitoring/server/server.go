package server

import (
	"context"
	"errors"
	"fmt"
	"github.com/golang/protobuf/ptypes/empty"
	apiPb "github.com/squzy/squzy_generated/generated/proto/v1"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"squzy/apps/squzy_application_monitoring/config"
	"squzy/apps/squzy_application_monitoring/database"
)

type server struct {
	db database.Database
	config config.Config
}

func (s *server) GetApplicationById(ctx context.Context, request *apiPb.GetApplicationByIdRequest) (*apiPb.Application, error) {
	panic("implement me")
}

func (s *server) GetApplicationList(ctx context.Context, e *empty.Empty) (*apiPb.GetApplicationListResponse, error) {
	panic("implement me")
}

var (
	errMissingName = errors.New("missing application name")
)

func (s *server) InitializeApplication(ctx context.Context, req *apiPb.ApplicationInfo) (*apiPb.InitializeApplicationResponse, error) {
	if req.Name == "" {
		return nil, errMissingName
	}

	application, err := s.db.FindOrCreate(ctx, req.Name, req.HostName)
	if err != nil {
		return nil, err
	}
	return &apiPb.InitializeApplicationResponse{
		ApplicationId: application.Id.Hex(),
		TracingHeader: s.config.GetTracingHeader(),
	}, nil
}

func (s *server) SaveTransaction(ctx context.Context, req *apiPb.TransactionInfo) (*empty.Empty, error) {
	applicationId, err := primitive.ObjectIDFromHex(req.ApplicationId)
	if err != nil {
		return nil, err
	}

	_, err = s.db.FindApplicationById(ctx, applicationId)
	if err != nil {
		return nil, err
	}

	// @TODO pass transaction info into storage
	fmt.Println(req)
	return &empty.Empty{}, nil
}

func New(db database.Database, config config.Config) apiPb.ApplicationMonitoringServer {
	return &server{
		db: db,
		config: config,
	}
}
