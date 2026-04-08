package config

import (
	"os"
	"strconv"
)

// Config는 애플리케이션 전체 설정을 담습니다.
type Config struct {
	Port      string
	JWTSecret string
	OpenStack OpenStackConfig
	Google    GoogleConfig
	SSH       *SSHConfig // nil이면 SSH 서버 비활성화
}

// OpenStackConfig는 OpenStack 연결에 필요한 설정입니다.
type OpenStackConfig struct {
	AuthURL     string
	Username    string
	Password    string
	ProjectName string
	DomainName  string
	ProjectID   string
	CFClientID  string
	CFSecret    string
}

// GoogleConfig는 Google OAuth 연결에 필요한 설정입니다.
type GoogleConfig struct {
	ClientID     string
	ClientSecret string
	RedirectURL  string
}

// SSHConfig는 SSH 릴레이 서버 설정입니다.
type SSHConfig struct {
	ListenPort      string
	HostKeyPath     string
	CAPublicKeyPath string
	Namespace       string
	ServiceKeyPath  string
	MenuPageSize    int
}

// Load는 환경변수에서 설정을 읽어 Config를 반환합니다.
func Load() *Config {
	jwtSecret := os.Getenv("RCP_JWT_SECRET")
	if jwtSecret == "" {
		jwtSecret = "default-low-security-key-for-dev" // #nosec G101
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	cfg := &Config{
		Port:      port,
		JWTSecret: jwtSecret,
		OpenStack: OpenStackConfig{
			AuthURL:     os.Getenv("OS_AUTH_URL"),
			Username:    os.Getenv("OS_USERNAME"),
			Password:    os.Getenv("OS_PASSWORD"),
			ProjectName: os.Getenv("OS_PROJECT_NAME"),
			DomainName:  os.Getenv("OS_USER_DOMAIN_NAME"),
			ProjectID:   os.Getenv("OS_PROJECT_ID"),
			CFClientID:  os.Getenv("CF_ACCESS_CLIENT_ID"),
			CFSecret:    os.Getenv("CF_ACCESS_CLIENT_SECRET"),
		},
		Google: GoogleConfig{
			ClientID:     os.Getenv("GG_OAUTH_CLIENT"),
			ClientSecret: os.Getenv("GG_OAUTH_SECRET"),
			RedirectURL:  os.Getenv("GG_REDIRECT_URL"),
		},
	}

	if sshPort := os.Getenv("SSH_LISTEN_PORT"); sshPort != "" {
		pageSize := 10
		if ps := os.Getenv("SSH_MENU_PAGE_SIZE"); ps != "" {
			if v, err := strconv.Atoi(ps); err == nil && v > 0 {
				pageSize = v
			}
		}
		cfg.SSH = &SSHConfig{
			ListenPort:      sshPort,
			HostKeyPath:     os.Getenv("SSH_HOST_KEY_PATH"),
			CAPublicKeyPath: os.Getenv("CF_CA_PUBLIC_KEY_PATH"),
			Namespace:       os.Getenv("QROUTER_NAMESPACE"),
			ServiceKeyPath:  os.Getenv("RCP_SERVICE_KEY_PATH"),
			MenuPageSize:    pageSize,
		}
	}

	return cfg
}
