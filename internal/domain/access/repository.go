package access

// TODO: DB 모델 확정 후 활성화, 아래는 예시
//
// type UserVM struct {
//     ID        int64
//     UserID    int64
//     VMID      string
//     VMName    string
//     VMIP      string
//     CreatedAt time.Time
// }
//
// type SSHRepository struct {
//     db *sql.DB
// }
//
// func NewSSHRepository(db *sql.DB) (*SSHRepository, error) {
//     schema := `CREATE TABLE IF NOT EXISTS user_vms (...);`
//     if _, err := db.Exec(schema); err != nil {
//         return nil, fmt.Errorf("failed to initialize user_vms schema: %w", err)
//     }
//     return &SSHRepository{db: db}, nil
// }
//
// func (r *SSHRepository) ListVMsByUser(ctx context.Context, userID int64) ([]UserVM, error) { ... }
// func (r *SSHRepository) FindVMByName(ctx context.Context, userID int64, vmName string) (*UserVM, error) { ... }
// func (r *SSHRepository) RegisterVM(ctx context.Context, vm *UserVM) error { ... }
// func (r *SSHRepository) RemoveVM(ctx context.Context, userID int64, vmID string) error { ... }
