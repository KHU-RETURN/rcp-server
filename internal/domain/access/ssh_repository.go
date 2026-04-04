package access

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// SSHRepository handles all DB I/O for the SSH relay domain.
// It also satisfies the compute.VMRegistrar interface so the compute domain
// can register and unregister VMs without importing this package directly.
type SSHRepository struct {
	db *sql.DB
}

// NewSSHRepository creates the user_vms table if it doesn't exist and returns an SSHRepository.
func NewSSHRepository(db *sql.DB) (*SSHRepository, error) {
	schema := `
	CREATE TABLE IF NOT EXISTS user_vms (
		id         INTEGER PRIMARY KEY AUTOINCREMENT,
		user_email TEXT NOT NULL,
		vm_name    TEXT NOT NULL,
		vm_id      TEXT UNIQUE NOT NULL,
		fixed_ip   TEXT NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(user_email, vm_name)
	);`
	if _, err := db.Exec(schema); err != nil {
		return nil, fmt.Errorf("failed to initialize user_vms schema: %w", err)
	}
	return &SSHRepository{db: db}, nil
}

// RegisterVM inserts a new user-VM mapping. Called by the compute domain after VM creation.
func (r *SSHRepository) RegisterVM(ctx context.Context, userEmail, vmName, vmID, fixedIP string) error {
	query := `INSERT INTO user_vms (user_email, vm_name, vm_id, fixed_ip) VALUES (?, ?, ?, ?)`
	_, err := r.db.ExecContext(ctx, query, userEmail, vmName, vmID, fixedIP)
	if err != nil {
		return fmt.Errorf("RegisterVM: %w", err)
	}
	return nil
}

// UnregisterVM removes a user-VM mapping by OpenStack VM ID. Called by the compute domain after VM deletion.
func (r *SSHRepository) UnregisterVM(ctx context.Context, vmID string) error {
	query := `DELETE FROM user_vms WHERE vm_id = ?`
	_, err := r.db.ExecContext(ctx, query, vmID)
	if err != nil {
		return fmt.Errorf("UnregisterVM: %w", err)
	}
	return nil
}

// ListVMsByEmail returns all VMs registered to a given user email.
func (r *SSHRepository) ListVMsByEmail(ctx context.Context, email string) ([]UserVM, error) {
	query := `SELECT id, user_email, vm_name, vm_id, fixed_ip, created_at FROM user_vms WHERE user_email = ?`
	rows, err := r.db.QueryContext(ctx, query, email)
	if err != nil {
		return nil, fmt.Errorf("ListVMsByEmail: %w", err)
	}
	defer rows.Close()

	var vms []UserVM
	for rows.Next() {
		var vm UserVM
		var createdAt string
		if err := rows.Scan(&vm.ID, &vm.UserEmail, &vm.VMName, &vm.VMID, &vm.FixedIP, &createdAt); err != nil {
			return nil, fmt.Errorf("ListVMsByEmail scan: %w", err)
		}
		vm.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt)
		vms = append(vms, vm)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("ListVMsByEmail rows: %w", err)
	}
	return vms, nil
}

// FindVMByEmailAndName returns the VM matching the given user email and VM name.
// Returns nil, nil if not found.
func (r *SSHRepository) FindVMByEmailAndName(ctx context.Context, email, vmName string) (*UserVM, error) {
	query := `SELECT id, user_email, vm_name, vm_id, fixed_ip, created_at FROM user_vms WHERE user_email = ? AND vm_name = ?`
	row := r.db.QueryRowContext(ctx, query, email, vmName)

	var vm UserVM
	var createdAt string
	err := row.Scan(&vm.ID, &vm.UserEmail, &vm.VMName, &vm.VMID, &vm.FixedIP, &createdAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("FindVMByEmailAndName: %w", err)
	}

	vm.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt)
	return &vm, nil
}
