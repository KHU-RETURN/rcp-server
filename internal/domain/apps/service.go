package apps

import (
	"context"
	"errors"
	"regexp"
	"strings"

	"github.com/google/uuid"
)

var validSubdomain = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)

type appRepo interface {
	SaveForInstance(ctx context.Context, ownerID uuid.UUID, instanceID string, app *App) (*App, error)
	DeleteByInstance(ctx context.Context, ownerID uuid.UUID, instanceID string) error
}

type Service struct {
	repo       appRepo
	baseDomain string
}

var (
	ErrHostRequired       = errors.New("host is required")
	ErrBaseDomainRequired = errors.New("app base domain is required")
	ErrSubdomainRequired  = errors.New("subdomain is required")
	ErrInvalidSubdomain   = errors.New("subdomain is invalid")
	ErrInstanceNotFound   = errors.New("instance not found")
	ErrAppNotFound        = errors.New("app not found")
	ErrAppAlreadyExists   = errors.New("app already exists for host or instance")
	ErrAppOperationFailed = errors.New("app operation failed")
)

func NewService(repo appRepo) *Service {
	return &Service{repo: repo, baseDomain: defaultAppBaseDomain()}
}

func NewServiceWithBaseDomain(repo appRepo, baseDomain string) *Service {
	return &Service{repo: repo, baseDomain: normalizeHost(baseDomain)}
}

func (s *Service) RegisterApp(ctx context.Context, ownerID uuid.UUID, instanceID string, req RegisterAppRequest) (*AppResponse, error) {
	app, err := s.validateRegisterAppRequest(instanceID, req)
	if err != nil {
		return nil, err
	}

	saved, err := s.repo.SaveForInstance(ctx, ownerID, instanceID, app)
	if err != nil {
		if errors.Is(err, ErrInstanceNotFound) || errors.Is(err, ErrAppAlreadyExists) {
			return nil, err
		}
		return nil, errors.Join(ErrAppOperationFailed, err)
	}
	saved.Subdomain = app.Subdomain
	res := appResponse(saved)
	return &res, nil
}

func (s *Service) DeleteApp(ctx context.Context, ownerID uuid.UUID, instanceID string) error {
	if err := s.repo.DeleteByInstance(ctx, ownerID, strings.TrimSpace(instanceID)); err != nil {
		if errors.Is(err, ErrAppNotFound) {
			return err
		}
		return errors.Join(ErrAppOperationFailed, err)
	}
	return nil
}

func (s *Service) validateRegisterAppRequest(instanceID string, req RegisterAppRequest) (*App, error) {
	baseDomain := normalizeHost(s.baseDomain)
	if baseDomain == "" {
		return nil, ErrBaseDomainRequired
	}
	subdomain := normalizeSubdomain(req.Subdomain)
	if subdomain == "" {
		return nil, ErrSubdomainRequired
	}
	if !validSubdomain.MatchString(subdomain) {
		return nil, ErrInvalidSubdomain
	}
	return &App{
		InstanceID: strings.TrimSpace(instanceID),
		Subdomain:  subdomain,
		Host:       subdomain + "." + baseDomain,
	}, nil
}

func normalizeSubdomain(subdomain string) string {
	return strings.TrimSpace(strings.ToLower(subdomain))
}

func firstLabel(host string) string {
	host = normalizeHost(host)
	label, _, _ := strings.Cut(host, ".")
	return label
}

func normalizeHost(host string) string {
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "" {
		return ""
	}
	if h, _, err := strings.Cut(host, ":"); err {
		return strings.ToLower(strings.TrimSpace(h))
	}
	return host
}
