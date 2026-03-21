package access

// CreateKeyPairRequest는 KeyPair 등록 요청 본문입니다.
type CreateKeyPairRequest struct {
	Name      string `json:"name" binding:"required"`
	PublicKey string `json:"public_key" binding:"required"`
}

// KeyPairResponse는 외부로 노출할 KeyPair 응답 규격입니다.
type KeyPairResponse struct {
	Name        string `json:"name"`
	Fingerprint string `json:"fingerprint"`
	PublicKey   string `json:"public_key"`
}
