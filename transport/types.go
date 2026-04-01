package transport

type NetType uint8

const (
	HTTP NetType = iota + 1
	TCP
	MICRO_SERVER
)
