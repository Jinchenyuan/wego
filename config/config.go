package config

import (
	"os"

	"github.com/pelletier/go-toml/v2"
)

type (
	config struct {
		Etcd       etcdCfg
		Http       httpCfg
		PostgreSQL postgreSQLCfg
		Redis      redisCfg
		Service    serviceCfg
		Services   servicesCfg
		Log        logCfg
		Profile    profileCfg
	}

	profileCfg struct {
		Name string
	}

	logCfg struct {
		Level string
	}

	etcdCfg struct {
		Endpoints []string
		User      string
		Password  string
	}

	httpCfg struct {
		Port             int
		ExcludeAuthPaths []string
	}

	postgreSQLCfg struct {
		DSN string
	}

	redisCfg struct {
		Addr     string
		Password string
		DB       int
	}

	serviceCfg struct {
		Name    string
		Version string
		Port    int
	}

	servicesCfg struct {
		Account string
		S3      string
	}
)

func Read(f string) (*config, error) {
	data, err := os.ReadFile(f)
	if err != nil {
		return nil, err
	}

	var cfg config
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}
