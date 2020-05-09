package application

import (
	"context"
	"github.com/shirou/gopsutil/host"
	apiPb "github.com/squzy/squzy_generated/generated/proto/v1"
	"google.golang.org/grpc"
	"io"
	"log"
	"os"
	"os/signal"
	"squzy/apps/agent_client/config"
	agent_executor "squzy/internal/agent-executor"
	"squzy/internal/helpers"
	"sync"
	"syscall"
)

type getSteamFn func(agent apiPb.AgentServerClient) (apiPb.AgentServer_SendMetricsClient, error)

type application struct {
	executor      agent_executor.AgentExecutor
	config        config.Config
	hostStatFn    func() (*host.InfoStat, error)
	getStreamFn   getSteamFn
	isStreamAvail bool
	buffer        []*apiPb.SendMetricsRequest
	client        apiPb.AgentServerClient
	interrupt     chan os.Signal
	mutex         sync.Mutex
}

func (a *application) getClient(opts ...grpc.DialOption) apiPb.AgentServerClient {
	for {
		ctx, cancel := helpers.TimeoutContext(context.Background(), 0)
		defer cancel()
		conn, err := grpc.DialContext(ctx, a.config.GetAgentServer(), opts...)
		if err == nil {
			return apiPb.NewAgentServerClient(conn)
		}
	}
}

func (a *application) register(hostStat *host.InfoStat) string {
	for {
		ctx, cancel := helpers.TimeoutContext(context.Background(), 0)
		defer cancel()
		res, err := a.client.Register(ctx, &apiPb.RegisterRequest{
			AgentName:  a.config.GetAgentName(),
			HostInfo: &apiPb.HostInfo{
				HostName: hostStat.Hostname,
				Os:       hostStat.OS,
				PlatformInfo: &apiPb.PlatformInfo{
					Name:    hostStat.Platform,
					Family:  hostStat.PlatformVersion,
					Version: hostStat.PlatformVersion,
				},
			},
		})

		if err == nil {
			return res.Id
		}
	}
}

func (a *application) getStream() apiPb.AgentServer_SendMetricsClient {
	for {
		s, err := a.getStreamFn(a.client)
		if err == nil {
			return s
		}
	}
}

func (a *application) Run() error {
	hostStat, err := a.hostStatFn()

	if err != nil {
		return err
	}

	a.client = a.getClient(grpc.WithInsecure(), grpc.WithBlock())

	agentId := a.register(hostStat)

	log.Printf("Registred with ID=%s", agentId)

	st := a.getStream()

	go func() {
		a.isStreamAvail = true

		for stat := range a.executor.Execute() {
			stat.AgentId = agentId
			stat.AgentName = a.config.GetAgentName()
			// what we should do if squzy server cant get msg
			err = st.Send(stat)

			if err == io.EOF {
				a.buffer = append(a.buffer, stat)
				if a.isStreamAvail {
					a.isStreamAvail = false
					go func() {
						st = a.getStream()
						a.isStreamAvail = true
						a.mutex.Lock()
						defer a.mutex.Unlock()
						for _, v := range a.buffer {
							_ = st.Send(v)
						}
						a.buffer = []*apiPb.SendMetricsRequest{}
					}()
				}
			}
		}
	}()

	// Wait signal from OS
	signal.Notify(a.interrupt, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(a.interrupt)

	<-a.interrupt

	_ = st.CloseSend()

	ctxClose, cancelClose := helpers.TimeoutContext(context.Background(), 0)
	defer cancelClose()

	_, _ = a.client.UnRegister(ctxClose, &apiPb.UnRegisterRequest{
		Id: agentId,
	})

	return nil
}

type Application interface {
	Run() error
}

func New(
	executor agent_executor.AgentExecutor,
	config config.Config,
	hostStatFn func() (*host.InfoStat, error),
	getStreamFn getSteamFn,
	interrupt chan os.Signal,
) Application {
	return &application{
		config:      config,
		executor:    executor,
		hostStatFn:  hostStatFn,
		getStreamFn: getStreamFn,
		interrupt:   interrupt,
	}
}

func NewStream(agent apiPb.AgentServerClient) (apiPb.AgentServer_SendMetricsClient, error) {
	return agent.SendMetrics(context.Background(), grpc.WaitForReady(true))
}