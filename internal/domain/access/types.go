package access

type KeyPair struct {
	Name        string
	Fingerprint string
	PublicKey   string
}

// StatusError — 인프라 레이어가 반환하며, 서비스 레이어가 gophercloud 없이 상태 코드로 분기할 수 있게 한다.
type StatusError struct {
	Code int
	Err  error
}

func (e *StatusError) Error() string { return e.Err.Error() }
func (e *StatusError) Unwrap() error { return e.Err }

// --- Request/Response DTOs ---

type CreateKeyPairRequest struct {
	Name      string `json:"name" binding:"required"`
	PublicKey string `json:"public_key" binding:"required"`
}

type KeyPairResponse struct {
	Name        string `json:"name"`
	Fingerprint string `json:"fingerprint"`
	PublicKey   string `json:"public_key"`
}
