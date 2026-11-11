package branding

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg" // DecodeConfig
	_ "image/png"  // DecodeConfig
	"net/http"
	"slices"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/apperror"
	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/audit"
	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/platform/clock"
	"github.com/digolitus/digolitus-statuspage/apps/backend/internal/platform/database"
)

// auditID is the fixed resource ID of the (single) branding in the audit log.
const auditID = "00000000-0000-4000-8000-00000000b4a0"

// Accepted image types per kind (detected from the content, not the header).
var imageTypes = map[string][]string{
	KindLogo:    {"image/png", "image/jpeg", "image/webp"},
	KindFavicon: {"image/png", "image/x-icon"},
}

const maxImageSide = 4096

// Upload describes an uploaded image.
type Upload struct {
	ContentType string    `json:"content_type"`
	SizeBytes   int       `json:"size_bytes"`
	URL         string    `json:"url"`
	UpdatedAt   time.Time `json:"updated_at"`
	sha         string
}

// Admin is the admin view: the settings, uploads, defaults and the hosts
// accepted for image URLs.
type Admin struct {
	Settings
	LogoUpload        *Upload    `json:"logo_upload"`
	FaviconUpload     *Upload    `json:"favicon_upload"`
	AllowedImageHosts []string   `json:"allowed_image_hosts"`
	Defaults          Settings   `json:"defaults"`
	UpdatedAt         *time.Time `json:"updated_at"`
}

// Link is a labelled URL.
type Link struct {
	Label string `json:"label"`
	URL   string `json:"url"`
}

// Public is what the public page and the login screen need.
type Public struct {
	Title             string     `json:"title"`
	Subtitle          string     `json:"subtitle"`
	Description       string     `json:"description"`
	LoginTitle        string     `json:"login_title"`
	LoginMessage      string     `json:"login_message"`
	FooterText        string     `json:"footer_text"`
	FooterLink        *Link      `json:"footer_link"`
	Font              string     `json:"font"`
	Colors            Colors     `json:"colors"`
	ShowTitleWithLogo bool       `json:"show_title_with_logo"`
	LogoURL           *string    `json:"logo_url"`
	FaviconURL        *string    `json:"favicon_url"`
	UpdatedAt         *time.Time `json:"updated_at"`
}

// Image is an uploaded file as served publicly.
type Image struct {
	ContentType string
	Data        []byte
	SHA256      string
}

// Service reads and changes the branding.
type Service struct {
	pool         *pgxpool.Pool
	now          clock.Clock
	siteName     string
	allowedHosts []string

	mu        sync.Mutex
	title     string
	titleRead time.Time
}

// NewService creates a Service. siteName (SITE_NAME) is the default title;
// allowedHosts are the hosts accepted for image URLs.
func NewService(pool *pgxpool.Pool, now clock.Clock, siteName string, allowedHosts []string) *Service {
	if allowedHosts == nil {
		allowedHosts = []string{}
	}
	return &Service{pool: pool, now: now, siteName: siteName, allowedHosts: allowedHosts}
}

func (s *Service) load(ctx context.Context, q database.Querier, lock bool) (Settings, *time.Time, error) {
	settings := Defaults(s.siteName)
	sql := "SELECT settings, updated_at FROM branding_settings"
	if lock {
		sql += " FOR UPDATE"
	}
	var raw []byte
	var at time.Time
	err := q.QueryRow(ctx, sql).Scan(&raw, &at)
	if errors.Is(err, pgx.ErrNoRows) {
		return settings, nil, nil
	}
	if err != nil {
		return Settings{}, nil, fmt.Errorf("load branding: %w", err)
	}
	// Fields added after the document was saved keep their defaults.
	if err := json.Unmarshal(raw, &settings); err != nil {
		return Settings{}, nil, fmt.Errorf("decode branding: %w", err)
	}
	return settings, &at, nil
}

func (s *Service) uploads(ctx context.Context, q database.Querier) (map[string]*Upload, error) {
	rows, err := q.Query(ctx, "SELECT kind, content_type, octet_length(data), sha256, updated_at FROM branding_images")
	if err != nil {
		return nil, fmt.Errorf("list branding images: %w", err)
	}
	defer rows.Close()
	out := map[string]*Upload{}
	for rows.Next() {
		var kind string
		u := &Upload{}
		if err := rows.Scan(&kind, &u.ContentType, &u.SizeBytes, &u.sha, &u.UpdatedAt); err != nil {
			return nil, err
		}
		u.URL = imagePath(kind, u.sha)
		out[kind] = u
	}
	return out, rows.Err()
}

func imagePath(kind, sha string) string {
	return "/api/v1/branding/images/" + kind + "?v=" + sha[:16]
}

// Public returns the identity for visitors.
func (s *Service) Public(ctx context.Context) (Public, error) {
	settings, at, err := s.load(ctx, s.pool, false)
	if err != nil {
		return Public{}, err
	}
	ups, err := s.uploads(ctx, s.pool)
	if err != nil {
		return Public{}, err
	}
	p := Public{Title: settings.Title, Subtitle: settings.Subtitle, Description: settings.Description,
		LoginTitle: settings.LoginTitle, LoginMessage: settings.LoginMessage, FooterText: settings.FooterText,
		Font: settings.Font, Colors: settings.Colors, ShowTitleWithLogo: settings.ShowTitleWithLogo, UpdatedAt: at}
	if settings.FooterLinkLabel != "" && settings.FooterLinkURL != "" {
		p.FooterLink = &Link{Label: settings.FooterLinkLabel, URL: settings.FooterLinkURL}
	}
	p.LogoURL = s.resolve(KindLogo, settings.Logo, ups)
	p.FaviconURL = s.resolve(KindFavicon, settings.Favicon, ups)
	return p, nil
}

// resolve returns the URL to show for an image, or nil. A URL whose host was
// removed from the allow-list is dropped: the CSP would block it anyway.
func (s *Service) resolve(kind string, c ImageChoice, ups map[string]*Upload) *string {
	switch {
	case c.Source == SourceUpload && ups[kind] != nil:
		u := ups[kind].URL
		return &u
	case c.Source == SourceURL && c.URL != nil && checkImageURL(*c.URL, s.allowedHosts) == "":
		u := *c.URL
		return &u
	}
	return nil
}

// Admin returns the admin view.
func (s *Service) Admin(ctx context.Context) (Admin, error) {
	return s.admin(ctx, s.pool)
}

func (s *Service) admin(ctx context.Context, q database.Querier) (Admin, error) {
	settings, at, err := s.load(ctx, q, false)
	if err != nil {
		return Admin{}, err
	}
	ups, err := s.uploads(ctx, q)
	if err != nil {
		return Admin{}, err
	}
	return Admin{Settings: settings, LogoUpload: ups[KindLogo], FaviconUpload: ups[KindFavicon],
		AllowedImageHosts: s.allowedHosts, Defaults: Defaults(s.siteName), UpdatedAt: at}, nil
}

// lockForUpdate creates the row when missing and locks it, so concurrent
// changes serialize.
func (s *Service) lockForUpdate(ctx context.Context, tx pgx.Tx) (Settings, *time.Time, error) {
	if _, err := tx.Exec(ctx, `INSERT INTO branding_settings (settings, updated_at) VALUES ($1, $2) ON CONFLICT (id) DO NOTHING`,
		Defaults(s.siteName), time.Unix(0, 0).UTC()); err != nil {
		return Settings{}, nil, fmt.Errorf("init branding: %w", err)
	}
	return s.load(ctx, tx, true)
}

func (s *Service) save(ctx context.Context, tx pgx.Tx, settings Settings, at time.Time) error {
	_, err := tx.Exec(ctx, "UPDATE branding_settings SET settings = $1, updated_at = $2", settings, at)
	if err != nil {
		return fmt.Errorf("save branding: %w", err)
	}
	s.forgetTitle()
	return nil
}

// Update replaces the settings.
func (s *Service) Update(ctx context.Context, in Settings) (Admin, error) {
	in.normalize()
	var out Admin
	err := database.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		before, _, err := s.lockForUpdate(ctx, tx)
		if err != nil {
			return err
		}
		ups, err := s.uploads(ctx, tx)
		if err != nil {
			return err
		}
		if err := in.validate(func(kind string) bool { return ups[kind] != nil }, s.allowedHosts); err != nil {
			return err
		}
		now := s.now()
		if err := s.save(ctx, tx, in, now); err != nil {
			return err
		}
		if err := audit.Record(ctx, tx, audit.ResourceBranding, auditID, "updated", audit.Diff(before, in), now); err != nil {
			return err
		}
		out, err = s.admin(ctx, tx)
		return err
	})
	return out, err
}

// PutImage stores an uploaded image and selects it as the kind's source.
func (s *Service) PutImage(ctx context.Context, kind string, data []byte) (Admin, error) {
	contentType, err := checkImage(kind, data)
	if err != nil {
		return Admin{}, err
	}
	sum := sha256.Sum256(data)
	sha := hex.EncodeToString(sum[:])
	var out Admin
	err = database.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		settings, _, err := s.lockForUpdate(ctx, tx)
		if err != nil {
			return err
		}
		now := s.now()
		if _, err := tx.Exec(ctx, `INSERT INTO branding_images (kind, content_type, data, sha256, updated_at) VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (kind) DO UPDATE SET content_type = EXCLUDED.content_type, data = EXCLUDED.data, sha256 = EXCLUDED.sha256, updated_at = EXCLUDED.updated_at`,
			kind, contentType, data, sha, now); err != nil {
			return fmt.Errorf("store branding image: %w", err)
		}
		choice := settings.choice(kind)
		*choice = ImageChoice{Source: SourceUpload}
		if err := s.save(ctx, tx, settings, now); err != nil {
			return err
		}
		after := map[string]any{"kind": kind, "content_type": contentType, "size_bytes": len(data), "sha256": sha}
		if err := audit.Record(ctx, tx, audit.ResourceBranding, auditID, "image_uploaded", audit.Changes{After: after}, now); err != nil {
			return err
		}
		out, err = s.admin(ctx, tx)
		return err
	})
	return out, err
}

// DeleteImage removes an uploaded image; a kind that used it shows none.
func (s *Service) DeleteImage(ctx context.Context, kind string) (Admin, error) {
	var out Admin
	err := database.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		settings, _, err := s.lockForUpdate(ctx, tx)
		if err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, "DELETE FROM branding_images WHERE kind = $1", kind)
		if err != nil {
			return fmt.Errorf("delete branding image: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return apperror.NotFound("branding_image")
		}
		now := s.now()
		if choice := settings.choice(kind); choice.Source == SourceUpload {
			*choice = ImageChoice{Source: SourceNone}
			if err := s.save(ctx, tx, settings, now); err != nil {
				return err
			}
		}
		if err := audit.Record(ctx, tx, audit.ResourceBranding, auditID, "image_removed", audit.Changes{Before: map[string]any{"kind": kind}}, now); err != nil {
			return err
		}
		out, err = s.admin(ctx, tx)
		return err
	})
	return out, err
}

func (s *Settings) choice(kind string) *ImageChoice {
	if kind == KindFavicon {
		return &s.Favicon
	}
	return &s.Logo
}

// Image returns an uploaded image.
func (s *Service) Image(ctx context.Context, kind string) (Image, error) {
	var img Image
	err := s.pool.QueryRow(ctx, "SELECT content_type, data, sha256 FROM branding_images WHERE kind = $1", kind).
		Scan(&img.ContentType, &img.Data, &img.SHA256)
	if errors.Is(err, pgx.ErrNoRows) {
		return Image{}, apperror.NotFound("branding_image")
	}
	if err != nil {
		return Image{}, fmt.Errorf("load branding image: %w", err)
	}
	return img, nil
}

// checkImage validates an upload and returns its detected content type.
func checkImage(kind string, data []byte) (string, error) {
	if len(data) == 0 {
		return "", apperror.Invalid("image_empty", "O arquivo está vazio.")
	}
	if len(data) > MaxImageBytes[kind] {
		return "", &apperror.Error{Kind: apperror.KindInvalid, Code: "payload_too_large",
			Message: fmt.Sprintf("A imagem passa de %d KB.", MaxImageBytes[kind]>>10)}
	}
	ct := http.DetectContentType(data)
	if !slices.Contains(imageTypes[kind], ct) {
		accepted := "PNG, JPG ou WebP"
		if kind == KindFavicon {
			accepted = "PNG ou ICO"
		}
		return "", apperror.Invalid("image_type_not_allowed", "Formato não aceito. Envie "+accepted+".")
	}
	if ct == "image/png" || ct == "image/jpeg" {
		cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
		if err != nil {
			return "", apperror.Invalid("image_invalid", "Não foi possível ler a imagem. Verifique o arquivo.")
		}
		if cfg.Width > maxImageSide || cfg.Height > maxImageSide {
			return "", apperror.Invalid("image_too_large", fmt.Sprintf("A imagem deve ter no máximo %d×%d pixels.", maxImageSide, maxImageSide))
		}
	}
	return ct, nil
}

// SiteName returns the configured title for e-mails and feeds. It is cached
// for a short time; errors fall back to the last value or SITE_NAME.
func (s *Service) SiteName() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.title != "" && s.now().Sub(s.titleRead) < 30*time.Second {
		return s.title
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	settings, _, err := s.load(ctx, s.pool, false)
	switch {
	case err == nil:
		s.title, s.titleRead = settings.Title, s.now()
	case s.title == "":
		return Defaults(s.siteName).Title
	}
	return s.title
}

func (s *Service) forgetTitle() {
	s.mu.Lock()
	s.title = ""
	s.mu.Unlock()
}
