package wego

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/Jinchenyuan/wego/logger"
	"github.com/Jinchenyuan/wego/pubsub"
	"github.com/Jinchenyuan/wego/reminder"
	"github.com/Jinchenyuan/wego/third_party/etcd"
	"github.com/Jinchenyuan/wego/transport"
	httptransport "github.com/Jinchenyuan/wego/transport/http"
	"github.com/Jinchenyuan/wego/transport/micro"
	"github.com/gin-gonic/gin"
	redis "github.com/redis/go-redis/v9"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/driver/pgdriver"
	"go-micro.dev/v5/registry"
	etcdReg "go-micro.dev/v5/registry/etcd"
)

var log *logger.Logger

var ErrRuntimeStarted = errors.New("mesa runtime already started")
var ErrReminderDBNotConfigured = errors.New("reminder db is not configured")
var ErrReminderNotifierNotConfigured = errors.New("reminder notifier is not configured")
var ErrEtcdNotConfigured = errors.New("etcd is not configured")

type RuntimeState string

const (
	StateNew      RuntimeState = "new"
	StateStarting RuntimeState = "starting"
	StateRunning  RuntimeState = "running"
	StateStopping RuntimeState = "stopping"
	StateStopped  RuntimeState = "stopped"
)

type HealthCheck struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

type HealthStatus struct {
	OK     bool                   `json:"ok"`
	State  RuntimeState           `json:"state"`
	Checks map[string]HealthCheck `json:"checks,omitempty"`
}

type stopper interface{ Stop(context.Context) error }

type Mesa struct {
	opts    options
	etcdCtl *etcd.Ctl
	DB      *bun.DB
	Redis   *redis.Client

	runtimeMu      sync.RWMutex
	state          RuntimeState
	runtimeCtx     context.Context
	runtimeCancel  context.CancelFunc
	components     []Component
	componentIndex map[string]Component
	servers        []transport.Server
	shutdownDone   chan struct{}
	shutdownErr    error
}

func New(opts ...Options) (*Mesa, error) {
	o := options{}
	for _, opt := range opts {
		opt(&o)
	}
	initLogger(o)

	m := &Mesa{opts: o, state: StateNew, componentIndex: make(map[string]Component), shutdownDone: make(chan struct{})}
	var err error
	if o.dsn != "" {
		m.DB, err = newDB(o.dsn)
		if err != nil {
			return nil, fmt.Errorf("connect postgres: %w", err)
		}
	}
	if o.RedisConfig.Addr != "" {
		m.Redis, err = newRedis(o.RedisConfig)
		if err != nil {
			_ = m.closeDB()
			return nil, fmt.Errorf("connect redis: %w", err)
		}
	}
	if len(o.EtcdConfig.Endpoints) > 0 {
		m.etcdCtl, err = etcd.NewCtl(etcd.ClientConfig{ConnectionType: etcd.ClientNonTLS}, etcd.WithEndpoints(o.EtcdConfig.Endpoints), etcd.WithAuth(o.EtcdConfig.Username, o.EtcdConfig.Password))
		if err != nil {
			_ = m.closeRedis()
			_ = m.closeDB()
			return nil, fmt.Errorf("connect etcd: %w", err)
		}
	}
	if o.HttpPort > 0 {
		hs := httptransport.NewHTTPServer(httptransport.WithHost(net.ParseIP("0.0.0.0")), httptransport.WithPort(o.HttpPort), httptransport.WithType(transport.HTTP))
		m.registerHealthRoutes(hs)
		m.servers = append(m.servers, hs)
	}
	if o.serviceScheme.Name != "" {
		if len(o.EtcdConfig.Endpoints) == 0 {
			m.closeResources()
			return nil, ErrEtcdNotConfigured
		}
		reg := etcdReg.NewEtcdRegistry(registry.Addrs(o.EtcdConfig.Endpoints...), etcdReg.Auth(o.EtcdConfig.Username, o.EtcdConfig.Password))
		m.servers = append(m.servers, micro.NewMicroServer(micro.WithRegistry(reg), micro.WithType(transport.MICRO_SERVER), micro.WithServiceScheme(o.serviceScheme)))
	}
	m.servers = append(m.servers, o.Servers...)
	if err := m.registerDefaultComponents(); err != nil {
		m.closeResources()
		return nil, err
	}
	SetGlobalMesa(m)
	return m, nil
}

func (m *Mesa) Run(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	m.runtimeMu.Lock()
	if m.state != "" && m.state != StateNew {
		m.runtimeMu.Unlock()
		return ErrRuntimeStarted
	}
	m.state = StateStarting
	signalCtx, stopSignals := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	m.runtimeCtx, m.runtimeCancel = context.WithCancel(signalCtx)
	servers := append([]transport.Server(nil), m.servers...)
	components := append([]Component(nil), m.components...)
	m.runtimeMu.Unlock()
	defer stopSignals()

	started := make([]any, 0, len(servers)+len(components))
	for _, server := range servers {
		if err := server.Start(m.runtimeCtx); err != nil {
			m.runtimeCancel()
			_ = stopItems(context.Background(), started)
			m.finishStopped()
			return fmt.Errorf("start server type %d: %w", server.GetType(), err)
		}
		started = append(started, server)
	}
	for _, component := range components {
		if err := component.Start(m.runtimeCtx); err != nil {
			m.runtimeCancel()
			_ = stopItems(context.Background(), started)
			m.finishStopped()
			return fmt.Errorf("start component %s: %w", component.Name(), err)
		}
		started = append(started, component)
	}
	m.runtimeMu.Lock()
	m.state = StateRunning
	m.runtimeMu.Unlock()
	<-m.runtimeCtx.Done()
	return m.Shutdown(context.Background())
}

func (m *Mesa) Shutdown(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	m.runtimeMu.Lock()
	if m.state == StateStopped {
		err := m.shutdownErr
		m.runtimeMu.Unlock()
		return err
	}
	if m.state == StateStopping {
		done := m.shutdownDone
		m.runtimeMu.Unlock()
		select {
		case <-done:
			return m.shutdownErr
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	m.state = StateStopping
	if m.runtimeCancel != nil {
		m.runtimeCancel()
	}
	items := make([]any, 0, len(m.components)+len(m.servers))
	for i := len(m.components) - 1; i >= 0; i-- {
		items = append(items, m.components[i])
	}
	for i := len(m.servers) - 1; i >= 0; i-- {
		items = append(items, m.servers[i])
	}
	m.runtimeMu.Unlock()

	err := stopItems(ctx, items)
	err = errors.Join(err, m.closeResources())
	m.runtimeMu.Lock()
	m.shutdownErr = err
	m.state = StateStopped
	close(m.shutdownDone)
	m.runtimeMu.Unlock()
	return err
}

func stopItems(ctx context.Context, items []any) error {
	var result error
	for _, item := range items {
		if err := ctx.Err(); err != nil {
			return errors.Join(result, err)
		}
		if s, ok := item.(stopper); ok {
			result = errors.Join(result, s.Stop(ctx))
		}
	}
	return result
}

func (m *Mesa) finishStopped() {
	m.runtimeMu.Lock()
	m.state = StateStopped
	select {
	case <-m.shutdownDone:
	default:
		close(m.shutdownDone)
	}
	m.runtimeMu.Unlock()
	_ = m.closeResources()
}

func (m *Mesa) State() RuntimeState { m.runtimeMu.RLock(); defer m.runtimeMu.RUnlock(); return m.state }
func (m *Mesa) Liveness() HealthStatus {
	state := m.State()
	return HealthStatus{OK: state != StateStopping && state != StateStopped, State: state}
}
func (m *Mesa) Readiness(ctx context.Context) HealthStatus {
	if ctx == nil {
		ctx = context.Background()
	}
	checkCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	m.runtimeMu.RLock()
	state, db, rdb, etcdCtl := m.state, m.DB, m.Redis, m.etcdCtl
	status := HealthStatus{OK: state == StateRunning, State: state, Checks: map[string]HealthCheck{}}
	check := func(name string, err error) {
		item := HealthCheck{OK: err == nil}
		if err != nil {
			item.Error = err.Error()
			status.OK = false
		}
		status.Checks[name] = item
	}
	if db != nil {
		check("postgres", db.PingContext(checkCtx))
	}
	if rdb != nil {
		check("redis", rdb.Ping(checkCtx).Err())
	}
	if etcdCtl != nil {
		_, err := etcdCtl.Get(checkCtx, "__wego_health__")
		check("etcd", err)
	}
	m.runtimeMu.RUnlock()
	return status
}

func (m *Mesa) registerHealthRoutes(server *httptransport.Server) {
	server.GetEngine().GET("/livez", func(c *gin.Context) {
		status := m.Liveness()
		code := 200
		if !status.OK {
			code = 503
		}
		c.JSON(code, status)
	})
	server.GetEngine().GET("/readyz", func(c *gin.Context) {
		status := m.Readiness(c.Request.Context())
		code := 200
		if !status.OK {
			code = 503
		}
		c.JSON(code, status)
	})
}

func (m *Mesa) GetServerByType(typ transport.NetType) transport.Server {
	m.runtimeMu.RLock()
	defer m.runtimeMu.RUnlock()
	for _, s := range m.servers {
		if s.GetType() == typ {
			return s
		}
	}
	return nil
}
func (m *Mesa) RegisterComponent(items ...Component) error {
	m.runtimeMu.Lock()
	defer m.runtimeMu.Unlock()
	if m.state != "" && m.state != StateNew {
		return ErrRuntimeStarted
	}
	for _, c := range items {
		if c == nil {
			return errors.New("component is nil")
		}
		name := normalizeComponentName(c.Name())
		if name == "" {
			return errors.New("component name is empty")
		}
		if _, ok := m.componentIndex[name]; ok {
			return fmt.Errorf("%w: %s", ErrComponentExists, name)
		}
		m.components = append(m.components, c)
		m.componentIndex[name] = c
	}
	return nil
}
func (m *Mesa) GetComponent(name string) (Component, bool) {
	if m == nil {
		return nil, false
	}
	m.runtimeMu.RLock()
	defer m.runtimeMu.RUnlock()
	c, ok := m.componentIndex[normalizeComponentName(name)]
	return c, ok
}
func (m *Mesa) MustGetComponent(name string) Component {
	c, ok := m.GetComponent(name)
	if !ok {
		panic(fmt.Sprintf("%v: %s", ErrComponentNotFound, name))
	}
	return c
}
func (m *Mesa) GetPubSub() (*pubsub.Service, bool) {
	return GetComponentAs[*pubsub.Service](m, "pubsub")
}
func (m *Mesa) MustGetPubSub() *pubsub.Service {
	return MustGetComponentAs[*pubsub.Service](m, "pubsub")
}
func (m *Mesa) GetReminder() (*reminder.Service, bool) {
	return GetComponentAs[*reminder.Service](m, "reminder")
}
func (m *Mesa) MustGetReminder() *reminder.Service {
	return MustGetComponentAs[*reminder.Service](m, "reminder")
}
func (m *Mesa) Components() []Component {
	if m == nil {
		return nil
	}
	m.runtimeMu.RLock()
	defer m.runtimeMu.RUnlock()
	return append([]Component(nil), m.components...)
}

func newDB(dsn string) (*bun.DB, error) {
	conn := pgdriver.NewConnector(pgdriver.WithDSN(dsn), pgdriver.WithDialTimeout(5*time.Second))
	sqldb := sql.OpenDB(conn)
	sqldb.SetMaxOpenConns(50)
	sqldb.SetMaxIdleConns(25)
	sqldb.SetConnMaxLifetime(30 * time.Minute)
	sqldb.SetConnMaxIdleTime(5 * time.Minute)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := sqldb.PingContext(ctx); err != nil {
		_ = sqldb.Close()
		return nil, err
	}
	return bun.NewDB(sqldb, pgdialect.New()), nil
}
func newRedis(cfg RedisConfig) (*redis.Client, error) {
	rdb := redis.NewClient(&redis.Options{Addr: cfg.Addr, Password: cfg.Password, DB: cfg.DB, DialTimeout: 5 * time.Second, ReadTimeout: 3 * time.Second, WriteTimeout: 3 * time.Second, PoolSize: 50, MinIdleConns: 10})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := rdb.Ping(ctx).Err(); err != nil {
		_ = rdb.Close()
		return nil, err
	}
	return rdb, nil
}
func (m *Mesa) closeDB() error {
	if m.DB != nil {
		err := m.DB.Close()
		m.DB = nil
		return err
	}
	return nil
}
func (m *Mesa) closeRedis() error {
	if m.Redis != nil {
		err := m.Redis.Close()
		m.Redis = nil
		return err
	}
	return nil
}
func (m *Mesa) closeResources() error {
	m.runtimeMu.Lock()
	defer m.runtimeMu.Unlock()
	return m.closeResourcesLocked()
}

func (m *Mesa) closeResourcesLocked() error {
	if m.etcdCtl != nil {
		m.etcdCtl.Close()
		m.etcdCtl = nil
	}
	return errors.Join(m.closeRedis(), m.closeDB())
}
func initLogger(opts options) {
	l := logger.GetLogger(opts.profile.Name)
	l.SetLevel(opts.LogLevel)
	SetGlobalLogger(l)
	log = l
}
func (m *Mesa) registerDefaultComponents() error {
	for _, registrar := range defaultComponentRegistrars {
		if registrar.enabled != nil && registrar.enabled(m.opts) {
			if err := registrar.register(m); err != nil {
				return fmt.Errorf("register default component %s: %w", registrar.name, err)
			}
		}
	}
	return nil
}
