package etcd

type ClientConnType int

const (
	ClientNonTLS       ClientConnType = iota
	ClientTLS                         // !!!TODO: support TLS
	ClientTLSAndNonTLS                // !!!TODO: support TLS And Non-TLS
)

type ClientConfig struct {
	ConnectionType ClientConnType
	CertAuthority  bool
	AutoTLS        bool
	RevokeCerts    bool
}

type AuthConfig struct {
	Username string
	Password string
}

func (cfg AuthConfig) Empty() bool {
	return cfg.Username == "" && cfg.Password == ""
}

type ClientOption func(any)
