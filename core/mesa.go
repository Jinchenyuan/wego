package core

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/Jinchenyuan/wego/core/logger"
	"github.com/Jinchenyuan/wego/core/transport"
	"github.com/Jinchenyuan/wego/core/transport/http"
	"github.com/Jinchenyuan/wego/core/transport/micro"
	"github.com/Jinchenyuan/wego/third_party/etcd"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	redis "github.com/redis/go-redis/v9"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/driver/pgdriver"
	"go-micro.dev/v5/registry"
	etcdReg "go-micro.dev/v5/registry/etcd"
)

var log *logger.Logger

type Mesa struct {
	opts          options
	retChan       chan int
	etcdCtl       *etcd.Ctl
	serversCtx    context.Context
	serversCancel context.CancelFunc
	DB            *bun.DB
	Redis         *redis.Client
}

func New(opts ...Options) *Mesa {
	o := options{}
	for _, opt := range opts {
		opt(&o)
	}

	etcdCtl, err := etcd.NewCtl(etcd.ClientConfig{
		ConnectionType: etcd.ClientNonTLS,
		CertAuthority:  false,
		AutoTLS:        false,
		RevokeCerts:    false,
	}, etcd.WithEndpoints(o.EtcdConfig.Endpoints), etcd.WithAuth(o.EtcdConfig.Username, o.EtcdConfig.Password))
	if err != nil {
		panic(fmt.Sprintf("failed to create etcd controller: %v", err))
	}

	db, err := newDB(o.dsn)
	if err != nil {
		panic(fmt.Sprintf("failed to connect to database: %v", err))
	}

	var rdb *redis.Client
	if o.RedisConfig.Addr != "" {
		rdb, err = newRedis(o.RedisConfig)
		if err != nil {
			panic(fmt.Sprintf("failed to connect to redis: %v", err))
		}
	}

	hs := http.NewHTTPServer(
		http.WithHost(net.ParseIP("0.0.0.0")),
		http.WithPort(o.HttpPort),
		http.WithType(transport.HTTP),
	)

	reg := etcdReg.NewEtcdRegistry(
		registry.Addrs(o.EtcdConfig.Endpoints...),
		etcdReg.Auth(o.EtcdConfig.Username, o.EtcdConfig.Password),
	)
	ms := micro.NewMicroServer(
		micro.WithRegistry(reg),
		micro.WithType(transport.MICRO_SERVER),
		micro.WithServiceScheme(o.serviceScheme),
	)
	WithServers(hs, ms)(&o)

	initLogger(o)

	m := &Mesa{
		opts:    o,
		retChan: make(chan int),
		etcdCtl: etcdCtl,
		DB:      db,
		Redis:   rdb,
	}

	SetGlobalMesa(m)

	return m
}

func (m *Mesa) Run() error {
	log.Info("start mesa!")

	m.startServers()

	go m.waitForStop()

	<-m.retChan

	return nil
}

func (m *Mesa) GetServerByType(typ transport.NetType) transport.Server {
	for _, server := range m.opts.Servers {
		s := server.GetType()
		if s == typ {
			return server
		}
	}
	return nil
}

func newDB(dsn string) (*bun.DB, error) {
	conn := pgdriver.NewConnector(
		pgdriver.WithDSN(dsn),
		pgdriver.WithDialTimeout(5*time.Second),
	)
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

	db := bun.NewDB(sqldb, pgdialect.New())
	return db, nil
}

func newRedis(cfg RedisConfig) (*redis.Client, error) {
	rdb := redis.NewClient(&redis.Options{
		Addr:         cfg.Addr,
		Password:     cfg.Password,
		DB:           cfg.DB,
		DialTimeout:  5 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
		PoolSize:     50,
		MinIdleConns: 10,
	})

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
		return m.DB.Close()
	}
	return nil
}

func (m *Mesa) closeRedis() error {
	if m.Redis != nil {
		return m.Redis.Close()
	}
	return nil
}

func (m *Mesa) startServers() {
	var wg sync.WaitGroup
	m.serversCtx, m.serversCancel = context.WithCancel(context.Background())

	for _, server := range m.opts.Servers {
		wg.Add(1)
		go func(s transport.Server) {
			defer wg.Done()
			if err := s.Start(m.serversCtx); err != nil {
				log.Error("server failed to start: %v\n", err)
				m.serversCancel() // Cancel context if any server fails
			}
		}(server)
	}

	wg.Wait()
	log.Info("All servers have started.")
}

func (m *Mesa) waitForStop() {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)

	sig := <-sigChan
	log.Info("Received signal: %v. Shutting down servers...\n", sig)

	m.serversCancel()

	m.etcdCtl.Close()

	m.closeDB()

	m.closeRedis()

	m.retChan <- 1

	log.Warn("Mesa has shut down.")
}

func initLogger(opts options) {
	l := logger.GetLogger(string(opts.profile.Name))
	l.SetLevel(opts.LogLevel)

	SetGlobalLogger(l)

	log = GetGlobalLogger()

}
