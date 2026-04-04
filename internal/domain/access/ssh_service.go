package access

import (
	"context"
	"errors"
	"net"

	gossh "golang.org/x/crypto/ssh"
)

var (
	ErrVMNotFound     = errors.New("vm not found")
	ErrVMAccessDenied = errors.New("vm access denied")
)

// sshVMRepository is the data-access interface for SSH relay VM operations.
// Defined here per project convention (interface in service layer).
type sshVMRepository interface {
	RegisterVM(ctx context.Context, userEmail, vmName, vmID, fixedIP string) error
	UnregisterVM(ctx context.Context, vmID string) error
	ListVMsByEmail(ctx context.Context, email string) ([]UserVM, error)
	FindVMByEmailAndName(ctx context.Context, email, vmName string) (*UserVM, error)
}

// SSHService holds the business logic for SSH relay operations.
type SSHService struct {
	repo         sshVMRepository
	dialer       *NamespaceDialer
	serviceKey   gossh.Signer
	menuPageSize int
}

// NewSSHService creates an SSHService with the given dependencies.
func NewSSHService(repo sshVMRepository, dialer *NamespaceDialer, serviceKey gossh.Signer, menuPageSize int) *SSHService {
	return &SSHService{
		repo:         repo,
		dialer:       dialer,
		serviceKey:   serviceKey,
		menuPageSize: menuPageSize,
	}
}

// MenuPageSize returns the number of VMs to display per page in the interactive menu.
func (s *SSHService) MenuPageSize() int {
	if s.menuPageSize <= 0 {
		return 10
	}
	return s.menuPageSize
}

// ListUserVMs returns all VMs accessible by the given user email.
func (s *SSHService) ListUserVMs(ctx context.Context, email string) ([]UserVM, error) {
	return s.repo.ListVMsByEmail(ctx, email)
}

// ResolveVM looks up a specific VM by user email and VM name.
// Returns ErrVMNotFound if the VM is not registered to this user.
func (s *SSHService) ResolveVM(ctx context.Context, email, vmName string) (*UserVM, error) {
	vm, err := s.repo.FindVMByEmailAndName(ctx, email, vmName)
	if err != nil {
		return nil, err
	}
	if vm == nil {
		return nil, ErrVMNotFound
	}
	return vm, nil
}

// DialVM opens a TCP connection to the VM's port 22 via the qrouter namespace.
func (s *SSHService) DialVM(ctx context.Context, vmIP string) (net.Conn, error) {
	return s.dialer.DialVM(ctx, vmIP)
}

// GetServiceKey returns the signer used to authenticate RCP to VMs.
func (s *SSHService) GetServiceKey() gossh.Signer {
	return s.serviceKey
}
