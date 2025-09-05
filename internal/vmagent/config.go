package vmagent

type Config struct {
	SFUForwardAddr string //send host udp for audio/video
	AvListenPort   string //listen on this port for audio/video to forward
	SFUgRPCAddr    string //for grpc connection for control input
}