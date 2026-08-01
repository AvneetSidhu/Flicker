package sfu

type Config struct {
	Port          string
	Tailnet       string
	LogLevel      string
	GRPCPort      string
	GRPCAuthToken string
	VMBaseName    string // libvirt domain name of the base VM to clone (e.g. "flicker-node")
}
