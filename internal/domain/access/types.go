package access

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
