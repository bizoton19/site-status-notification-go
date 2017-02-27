package config

type SMTPConfig struct {
	Hostname string
	Password string
	Username string
	Port     string
	From     string
	To       []string
}
