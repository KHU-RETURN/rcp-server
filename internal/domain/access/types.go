package access

// KeyPair는 SSH 키 페어의 도메인 표현입니다.
type KeyPair struct {
	Name        string
	Fingerprint string
	PublicKey   string
}

// StatusError는 HTTP 상태 코드를 담은 업스트림 에러입니다.
// 인프라 레이어가 반환하며, 서비스 레이어가 gophercloud 없이 상태 코드로 분기할 수 있게 합니다.
type StatusError struct {
	Code int
	Err  error
}

func (e *StatusError) Error() string { return e.Err.Error() }
func (e *StatusError) Unwrap() error { return e.Err }

// VMInfo는 SSH 릴레이에서 사용하는 VM 정보입니다.
type VMInfo struct {
	ID     string
	Name   string
	Status string
	IP     string // 사설 IP (qrouter 경유)
}

// CreateKeyPairRequest는 KeyPair 등록 요청 본문입니다.
type CreateKeyPairRequest struct {
	// OpenStack key pair name.
	Name string `json:"name" binding:"required"`
	// SSH public key in authorized_keys format.
	PublicKey string `json:"public_key" binding:"required"`
}

// KeyPairResponse는 외부로 노출할 KeyPair 응답 규격입니다.
type KeyPairResponse struct {
	// Key pair name.
	Name string `json:"name"`
	// OpenStack-generated fingerprint.
	Fingerprint string `json:"fingerprint"`
	// Public key stored in OpenStack.
	PublicKey string `json:"public_key"`
}
