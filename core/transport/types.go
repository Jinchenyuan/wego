package transport

type NetType uint8

const (
	HTTP NetType = iota + 1
	MICRO_SERVER
)

type ServiceType string

const (
	Account ServiceType = "account"
	Admin   ServiceType = "admin"
	S3      ServiceType = "s3"
)
