package functions

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
	"github.com/tetratelabs/wazero/sys"
)

const (
	MaxWasmBytes        = 32 << 20
	MaxSourceBytes      = 256 << 10
	MaxInputBytes       = 128 << 10
	MaxOutputBytes      = 128 << 10
	MaxHTTPBodyBytes    = 64 << 10
	MaxFunctionsPerUser = 20
	InvocationTimeout   = 10 * time.Second
)

var (
	ErrInvalidName      = errors.New("invalid function name")
	ErrInvalidWasm      = errors.New("invalid WASI module")
	ErrNotFound         = errors.New("function not found")
	ErrConflict         = errors.New("function name already exists")
	ErrLimit            = errors.New("function limit reached")
	ErrBusy             = errors.New("function runner is busy")
	ErrTimeout          = errors.New("function execution timed out")
	ErrOutputLimit      = errors.New("function output limit exceeded")
	ErrInvalidInput     = errors.New("function input must be a JSON object")
	ErrInvalidOutput    = errors.New("function output must be a JSON object")
	ErrInvalidLanguage  = errors.New("invalid function language")
	ErrBuildUnavailable = errors.New("function builder unavailable")
	ErrBuildFailed      = errors.New("function build failed")
	validName           = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)
)

type repository interface {
	List(context.Context, uuid.UUID) ([]Function, error)
	Get(context.Context, uuid.UUID, uuid.UUID) (*Function, error)
	GetPublic(context.Context, uuid.UUID) (*Function, error)
	SetKey(context.Context, uuid.UUID, uuid.UUID, []byte, time.Time, time.Time) (*Function, error)
	DeleteKey(context.Context, uuid.UUID, uuid.UUID) (*Function, error)
	Create(context.Context, uuid.UUID, string, []byte) (*Function, error)
	Update(context.Context, uuid.UUID, uuid.UUID, []byte) (*Function, error)
	CreateDeployment(context.Context, uuid.UUID, string, Deployment) (*Function, error)
	UpdateDeployment(context.Context, uuid.UUID, uuid.UUID, Deployment) (*Function, error)
	Delete(context.Context, uuid.UUID, uuid.UUID) error
}

type Service struct {
	repo     repository
	runtime  wazero.Runtime
	slots    chan struct{}
	createMu sync.Mutex
	builder  Builder
	data     *DataStore
	now      func() time.Time
}

func NewService(ctx context.Context, repo repository) (*Service, error) {
	config := wazero.NewRuntimeConfig().WithMemoryLimitPages(4096).WithCloseOnContextDone(true)
	runtime := wazero.NewRuntimeWithConfig(ctx, config)
	if _, err := wasi_snapshot_preview1.Instantiate(ctx, runtime); err != nil {
		_ = runtime.Close(ctx)
		return nil, err
	}
	return &Service{repo: repo, runtime: runtime, slots: make(chan struct{}, 4), now: time.Now}, nil
}

func (s *Service) SetBuilder(builder Builder)   { s.builder = builder }
func (s *Service) SetDataStore(data *DataStore) { s.data = data }

func enabled(value []bool) bool { return len(value) > 0 && value[0] }

func (s *Service) build(ctx context.Context, language string, source []byte) (Deployment, error) {
	if language != "rust" && language != "go" && language != "javascript" && language != "python" {
		return Deployment{}, ErrInvalidLanguage
	}
	if len(source) == 0 || len(source) > MaxSourceBytes {
		return Deployment{}, fmt.Errorf("source must be 1 to %d bytes", MaxSourceBytes)
	}
	if s.builder == nil {
		return Deployment{}, ErrBuildUnavailable
	}
	wasm, err := s.builder.Build(ctx, language, source)
	if err != nil {
		return Deployment{}, err
	}
	if err := s.validate(ctx, wasm); err != nil {
		return Deployment{}, err
	}
	return Deployment{Language: language, Source: source, Wasm: wasm}, nil
}

func (s *Service) CreateSource(ctx context.Context, owner uuid.UUID, name, language string, source []byte, dataMode ...bool) (*Function, error) {
	name = strings.TrimSpace(strings.ToLower(name))
	if !validName.MatchString(name) {
		return nil, ErrInvalidName
	}
	deployment, err := s.build(ctx, language, source)
	if err != nil {
		return nil, err
	}
	deployment.DataMode = enabled(dataMode)
	if deployment.DataMode && s.data == nil {
		return nil, ErrDataUnavailable
	}
	s.createMu.Lock()
	defer s.createMu.Unlock()
	items, err := s.repo.List(ctx, owner)
	if err != nil {
		return nil, err
	}
	if len(items) >= MaxFunctionsPerUser {
		return nil, ErrLimit
	}
	return s.repo.CreateDeployment(ctx, owner, name, deployment)
}

func (s *Service) UpdateSource(ctx context.Context, owner, id uuid.UUID, language string, source []byte, dataMode ...bool) (*Function, error) {
	if _, err := s.repo.Get(ctx, owner, id); err != nil {
		return nil, err
	}
	deployment, err := s.build(ctx, language, source)
	if err != nil {
		return nil, err
	}
	deployment.DataMode = enabled(dataMode)
	if deployment.DataMode && s.data == nil {
		return nil, ErrDataUnavailable
	}
	return s.repo.UpdateDeployment(ctx, owner, id, deployment)
}

func (s *Service) Close(ctx context.Context) error {
	err := s.runtime.Close(ctx)
	if s.data != nil {
		return errors.Join(err, s.data.Close())
	}
	return err
}

func (s *Service) List(ctx context.Context, owner uuid.UUID) ([]Function, error) {
	return s.repo.List(ctx, owner)
}

func (s *Service) Create(ctx context.Context, owner uuid.UUID, name string, wasm []byte, dataMode ...bool) (*Function, error) {
	name = strings.TrimSpace(strings.ToLower(name))
	if !validName.MatchString(name) {
		return nil, ErrInvalidName
	}
	if err := s.validate(ctx, wasm); err != nil {
		return nil, err
	}
	s.createMu.Lock()
	defer s.createMu.Unlock()
	items, err := s.repo.List(ctx, owner)
	if err != nil {
		return nil, err
	}
	if len(items) >= MaxFunctionsPerUser {
		return nil, ErrLimit
	}
	if enabled(dataMode) && s.data == nil {
		return nil, ErrDataUnavailable
	}
	return s.repo.CreateDeployment(ctx, owner, name, Deployment{Language: "wasm", Wasm: wasm, DataMode: enabled(dataMode)})
}

func (s *Service) Update(ctx context.Context, owner, id uuid.UUID, wasm []byte, dataMode ...bool) (*Function, error) {
	if err := s.validate(ctx, wasm); err != nil {
		return nil, err
	}
	if enabled(dataMode) && s.data == nil {
		return nil, ErrDataUnavailable
	}
	return s.repo.UpdateDeployment(ctx, owner, id, Deployment{Language: "wasm", Wasm: wasm, DataMode: enabled(dataMode)})
}

func (s *Service) Delete(ctx context.Context, owner, id uuid.UUID) error {
	if err := s.repo.Delete(ctx, owner, id); err != nil {
		return err
	}
	if s.data != nil {
		return s.data.DeleteFunction(ctx, owner, id)
	}
	return nil
}

func (s *Service) dataFor(ctx context.Context, owner, id uuid.UUID) (*DataStore, error) {
	if _, err := s.repo.Get(ctx, owner, id); err != nil {
		return nil, err
	}
	if s.data == nil {
		return nil, ErrDataUnavailable
	}
	return s.data, nil
}

func (s *Service) ListData(ctx context.Context, owner, id uuid.UUID, collection string, offset int) ([]DataItem, error) {
	data, err := s.dataFor(ctx, owner, id)
	if err != nil {
		return nil, err
	}
	return data.List(ctx, owner, id, collection, offset)
}

func (s *Service) GetData(ctx context.Context, owner, id uuid.UUID, collection, key string) (*DataItem, error) {
	data, err := s.dataFor(ctx, owner, id)
	if err != nil {
		return nil, err
	}
	return data.Get(ctx, owner, id, collection, key)
}

func (s *Service) PutData(ctx context.Context, owner, id uuid.UUID, collection, key string, value json.RawMessage) (*DataItem, error) {
	data, err := s.dataFor(ctx, owner, id)
	if err != nil {
		return nil, err
	}
	return data.Put(ctx, owner, id, collection, key, value)
}

func (s *Service) DeleteData(ctx context.Context, owner, id uuid.UUID, collection, key string) error {
	data, err := s.dataFor(ctx, owner, id)
	if err != nil {
		return err
	}
	return data.Delete(ctx, owner, id, collection, key)
}

func (s *Service) validate(ctx context.Context, wasm []byte) error {
	if len(wasm) == 0 || len(wasm) > MaxWasmBytes {
		return ErrInvalidWasm
	}
	select {
	case s.slots <- struct{}{}:
		defer func() { <-s.slots }()
	default:
		return ErrBusy
	}
	ctx, cancel := context.WithTimeout(ctx, InvocationTimeout)
	defer cancel()
	compiled, err := s.runtime.CompileModule(ctx, wasm)
	if err != nil {
		if ctx.Err() != nil {
			return ErrTimeout
		}
		return fmt.Errorf("%w: %v", ErrInvalidWasm, err)
	}
	defer func() { _ = compiled.Close(ctx) }()
	wasi := s.runtime.Module("wasi_snapshot_preview1")
	for _, imported := range compiled.ImportedFunctions() {
		module, name, _ := imported.Import()
		if module != "wasi_snapshot_preview1" {
			return ErrInvalidWasm
		}
		exported, ok := wasi.ExportedFunctionDefinitions()[name]
		if !ok || !slices.Equal(imported.ParamTypes(), exported.ParamTypes()) ||
			!slices.Equal(imported.ResultTypes(), exported.ResultTypes()) {
			return ErrInvalidWasm
		}
	}
	if len(compiled.ImportedMemories()) != 0 {
		return ErrInvalidWasm
	}
	if len(compiled.ExportedFunctions()) == 0 {
		return ErrInvalidWasm
	}
	start, ok := compiled.ExportedFunctions()["_start"]
	if !ok || len(start.ParamTypes()) != 0 || len(start.ResultTypes()) != 0 {
		return ErrInvalidWasm
	}
	return nil
}

func (s *Service) Invoke(ctx context.Context, owner, id uuid.UUID, input []byte) (*Result, error) {
	fn, err := s.repo.Get(ctx, owner, id)
	if err != nil {
		return nil, err
	}
	return s.execute(ctx, fn, input)
}

func (s *Service) InvokePublic(ctx context.Context, id uuid.UUID, key string, input []byte) (*Result, error) {
	fn, err := s.repo.GetPublic(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := s.checkKey(fn, key); err != nil {
		return nil, err
	}
	return s.execute(ctx, fn, input)
}

func (s *Service) execute(ctx context.Context, fn *Function, input []byte) (*Result, error) {
	if len(input) > MaxInputBytes {
		return nil, fmt.Errorf("input exceeds %d bytes", MaxInputBytes)
	}
	if !jsonObject(input) {
		return nil, ErrInvalidInput
	}
	select {
	case s.slots <- struct{}{}:
		defer func() { <-s.slots }()
	default:
		return nil, ErrBusy
	}
	ctx, cancel := context.WithTimeout(ctx, InvocationTimeout)
	defer cancel()
	compiled, err := s.runtime.CompileModule(ctx, fn.Wasm)
	if err != nil {
		return nil, fmt.Errorf("compile function: %w", err)
	}
	defer func() { _ = compiled.Close(context.Background()) }()
	stdout, stderr := &limitedWriter{}, &limitedWriter{}
	var stdin io.Reader = bytes.NewReader(input)
	var output io.Writer = stdout
	var protocol *dataOutput
	if fn.DataMode {
		if s.data == nil || fn.OwnerID == uuid.Nil {
			return nil, ErrDataUnavailable
		}
		stream := &dataInput{ctx: ctx, replies: make(chan []byte, maxDataOperationsPerInvocation+1)}
		var compact bytes.Buffer
		if err := json.Compact(&compact, input); err != nil {
			return nil, ErrInvalidInput
		}
		stdin = io.MultiReader(bytes.NewReader(append(compact.Bytes(), '\n')), stream)
		protocol = &dataOutput{ctx: ctx, store: s.data, owner: fn.OwnerID, function: fn.ID, input: stream}
		output = protocol
	}
	config := wazero.NewModuleConfig().WithStdin(stdin).WithStdout(output).WithStderr(stderr)
	if fn.Language == "python" {
		config = config.WithArgs("python", "-c", string(fn.Source))
	}
	_, err = s.runtime.InstantiateModule(ctx, compiled, config)
	if ctx.Err() != nil {
		return nil, ErrTimeout
	}
	if protocol != nil {
		_, finishErr := protocol.Finish()
		if finishErr != nil {
			return nil, finishErr
		}
		stdout = &protocol.final
	}
	if stdout.exceeded || stderr.exceeded {
		return nil, ErrOutputLimit
	}
	result := &Result{Stdout: stdout.String(), Stderr: stderr.String()}
	if err != nil {
		if exit, ok := errors.AsType[*sys.ExitError](err); ok {
			result.ExitCode = exit.ExitCode()
			return result, nil
		}
		return nil, fmt.Errorf("execute function: %w", err)
	}
	if !jsonObject(stdout.Bytes()) {
		return nil, ErrInvalidOutput
	}
	return result, nil
}

func jsonObject(data []byte) bool {
	data = bytes.TrimSpace(data)
	return len(data) > 0 && data[0] == '{' && json.Valid(data)
}

type limitedWriter struct {
	bytes.Buffer
	exceeded bool
}

func (w *limitedWriter) Write(p []byte) (int, error) {
	if w.Len()+len(p) > MaxOutputBytes {
		w.exceeded = true
		return 0, ErrOutputLimit
	}
	return w.Buffer.Write(p)
}
