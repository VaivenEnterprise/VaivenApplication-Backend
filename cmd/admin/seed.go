package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	authdomain "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/auth/domain"
	authusecase "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/auth/usecase"
	bookingsdomain "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/bookings/domain"
	bookingsusecase "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/bookings/usecase"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/config"
	imagesdomain "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/images/domain"
	itemsdomain "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/items/domain"
	itemsusecase "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/items/usecase"
)

// seedCredentialsEnv apunta al archivo `correo=contraseña` (fuera del
// repositorio) con una contraseña distinta por cuenta (spec 012, P-12).
const seedCredentialsEnv = "SEED_CREDENTIALS_FILE"

// seedTimeout limita el seed base; --scale suma tiempo por copia.
const (
	seedTimeout         = 10 * time.Minute
	seedTimeoutPerScale = 30 * time.Second
	maxSeedScale        = 1000
	maxCredentialsBytes = 64 << 10
)

// Puertos que usa el seed: los casos de uso reales los cumplen.
type (
	seedAuth interface {
		Register(ctx context.Context, in authusecase.RegisterInput) (authusecase.AuthResult, error)
	}
	seedUsers interface {
		ByEmail(ctx context.Context, email string) (authdomain.User, error)
	}
	seedItemsService interface {
		Create(ctx context.Context, viewer itemsusecase.Viewer, in itemsusecase.ItemInput) (itemsusecase.ItemView, error)
		ListMine(ctx context.Context, viewer itemsusecase.Viewer, q itemsusecase.ListQuery) (itemsusecase.Page, error)
	}
	seedBookingsService interface {
		Quote(ctx context.Context, viewer bookingsusecase.Viewer, in bookingsusecase.QuoteInput) (bookingsdomain.Quote, error)
		Create(ctx context.Context, viewer bookingsusecase.Viewer, in bookingsusecase.CreateInput) (bookingsusecase.BookingView, error)
		Accept(ctx context.Context, viewer bookingsusecase.Viewer, id string, meetingPoint *string) (bookingsusecase.BookingView, error)
		Cancel(ctx context.Context, viewer bookingsusecase.Viewer, id string, reason *string) (bookingsusecase.BookingView, error)
		List(ctx context.Context, viewer bookingsusecase.Viewer, q bookingsusecase.ListQuery) (bookingsusecase.Page, error)
	}
	// photoStore sube una foto de objeto como lo haría la app (firma de
	// images.Service) y la borra si algo falla después (CA-09).
	photoStore interface {
		Upload(ctx context.Context, ownerID string, data []byte) (imagesdomain.Ref, error)
		Destroy(ctx context.Context, ref imagesdomain.Ref) error
	}
)

// seedDeps son las dependencias del seed; close cierra la conexión.
type seedDeps struct {
	auth     seedAuth
	users    seedUsers
	admins   adminDeps
	items    seedItemsService
	bookings seedBookingsService
	photos   photoStore
	today    func() bookingsdomain.Date
	now      func() time.Time
	close    func()
}

// seedFactory arma las dependencias. local = fotos sin Cloudinary (solo
// development y test, para --scale).
type seedFactory func(ctx context.Context, cfg config.Config, local bool) (seedDeps, error)

// seedStats cuenta lo creado y lo que ya existía.
type seedStats struct{ created, existing int }

// runSeed es el subcomando completo (CA-02): guardas, credenciales, conexión
// y datos. Devuelve el código de salida; la conexión se cierra antes.
func runSeed(args []string, stdout, stderr io.Writer, cfg config.Config, getenv func(string) string, open seedFactory) int {
	fs := flag.NewFlagSet("seed", flag.ContinueOnError)
	fs.SetOutput(stderr)
	scale := fs.Int("scale", 0, "copias extra del conjunto para pruebas de rendimiento (solo development y test)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() > 0 || *scale < 0 || *scale > maxSeedScale {
		_, _ = fmt.Fprintf(stderr, "error: uso: admin seed [--scale N], con 0 ≤ N ≤ %d\n", maxSeedScale)
		return 2
	}
	// Guardas antes de leer credenciales o conectar: nada se escribe.
	if cfg.IsProduction() {
		_, _ = fmt.Fprintln(stderr, "error: seed no se ejecuta con APP_ENV=production (spec 012, RF-02b)")
		return 1
	}
	if *scale > 0 && cfg.AppEnv != config.EnvDevelopment && cfg.AppEnv != config.EnvTest {
		_, _ = fmt.Fprintln(stderr, "error: --scale solo se permite con APP_ENV=development o test")
		return 1
	}
	creds, err := loadSeedCredentials(getenv(seedCredentialsEnv), seedRequiredEmails())
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}

	ctx, cancel := context.WithTimeout(context.Background(), seedTimeout+time.Duration(*scale)*seedTimeoutPerScale)
	defer cancel()
	d, err := open(ctx, cfg, *scale > 0)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "error: %s\n", redact(err.Error(), cfg.MongoURI))
		return 1
	}
	defer d.close()

	s := &seeder{d: d, out: stdout}
	if err := s.base(ctx, creds); err != nil {
		_, _ = fmt.Fprintf(stderr, "error: %s\n", redact(err.Error(), cfg.MongoURI))
		return 1
	}
	for k := 1; k <= *scale; k++ {
		if err := s.copy(ctx, k); err != nil {
			_, _ = fmt.Fprintf(stderr, "error en la copia %d: %s\n", k, redact(err.Error(), cfg.MongoURI))
			return 1
		}
	}
	_, _ = fmt.Fprintf(stdout, "\nseed: personas %d nuevas/%d existentes, objetos %d/%d, reservas %d/%d\n",
		s.people.created, s.people.existing, s.objects.created, s.objects.existing, s.reservations.created, s.reservations.existing)
	return 0
}

func seedRequiredEmails() []string {
	out := []string{authdomain.NormalizeEmail(seedAdminEmail)}
	for _, p := range seedPeople {
		out = append(out, authdomain.NormalizeEmail(p.Email))
	}
	return out
}

// loadSeedCredentials lee `correo=contraseña` por línea (se ignoran las
// vacías y las que empiezan con #). Exige exactamente los correos required,
// una contraseña por cuenta que cumpla la política y ninguna repetida. Los
// errores nunca incluyen contraseñas.
func loadSeedCredentials(path string, required []string) (map[string]string, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("define %s con la ruta del archivo de credenciales (fuera del repositorio)", seedCredentialsEnv)
	}
	f, err := os.Open(path) // #nosec G304 -- ruta elegida por quien opera, solo lectura
	if err != nil {
		return nil, fmt.Errorf("no se pudo abrir %s: %w", seedCredentialsEnv, err)
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(io.LimitReader(f, maxCredentialsBytes+1))
	if err != nil {
		return nil, fmt.Errorf("no se pudo leer %s: %w", seedCredentialsEnv, err)
	}
	if len(data) > maxCredentialsBytes {
		return nil, fmt.Errorf("%s es demasiado grande", seedCredentialsEnv)
	}
	want := map[string]bool{}
	for _, e := range required {
		want[e] = true
	}
	creds := map[string]string{}
	sc := bufio.NewScanner(bytes.NewReader(data))
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimRight(sc.Text(), "\r")
		if strings.TrimSpace(line) == "" || strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		email, pwd, ok := strings.Cut(line, "=")
		if !ok {
			return nil, fmt.Errorf("%s, línea %d: se espera correo=contraseña", seedCredentialsEnv, n)
		}
		email = authdomain.NormalizeEmail(strings.TrimSpace(email))
		if !want[email] {
			return nil, fmt.Errorf("%s, línea %d: %s no es una cuenta del seed", seedCredentialsEnv, n, email)
		}
		if _, dup := creds[email]; dup {
			return nil, fmt.Errorf("%s, línea %d: %s aparece dos veces", seedCredentialsEnv, n, email)
		}
		if problems := authdomain.ValidatePassword(pwd); len(problems) > 0 {
			return nil, fmt.Errorf("%s, línea %d: la contraseña de %s %s", seedCredentialsEnv, n, email, problems[0].Issue)
		}
		creds[email] = pwd
	}
	var missing []string
	for _, e := range required {
		if _, ok := creds[e]; !ok {
			missing = append(missing, e)
		}
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("%s: faltan las cuentas %s", seedCredentialsEnv, strings.Join(missing, ", "))
	}
	byPwd := map[string]string{}
	emails := make([]string, 0, len(creds))
	for e := range creds {
		emails = append(emails, e)
	}
	sort.Strings(emails)
	for _, e := range emails {
		if other, ok := byPwd[creds[e]]; ok {
			return nil, fmt.Errorf("%s: %s y %s comparten contraseña; cada cuenta debe tener la suya", seedCredentialsEnv, other, e)
		}
		byPwd[creds[e]] = e
	}
	return creds, nil
}

// seeder crea los datos de forma idempotente: busca antes de crear y nunca
// sobrescribe (spec 012, P-13).
type seeder struct {
	d                             seedDeps
	out                           io.Writer
	people, objects, reservations seedStats
}

func (s *seeder) logf(format string, a ...any) { _, _ = fmt.Fprintf(s.out, format+"\n", a...) }

// base crea el conjunto de la demostración con las credenciales del archivo.
func (s *seeder) base(ctx context.Context, creds map[string]string) error {
	if err := s.admin(ctx, creds[authdomain.NormalizeEmail(seedAdminEmail)]); err != nil {
		return err
	}
	pwd := func(p seedPerson) (string, error) { return creds[authdomain.NormalizeEmail(p.Email)], nil }
	return s.dataset(ctx, seedPeople, "", pwd)
}

// copy crea la copia k del conjunto para pruebas de rendimiento: correos con
// +s<k> y contraseñas aleatorias que no se guardan ni se muestran (esas
// cuentas no se usan para iniciar sesión).
func (s *seeder) copy(ctx context.Context, k int) error {
	people := make([]seedPerson, len(seedPeople))
	for i, p := range seedPeople {
		local, domain, _ := strings.Cut(p.Email, "@")
		people[i] = seedPerson{Email: fmt.Sprintf("%s+s%d@%s", local, k, domain), DisplayName: fmt.Sprintf("%s %d", p.DisplayName, k)}
	}
	return s.dataset(ctx, people, fmt.Sprintf(" #%d", k), func(seedPerson) (string, error) { return randomPassword() })
}

func (s *seeder) dataset(ctx context.Context, people []seedPerson, suffix string, pwd func(seedPerson) (string, error)) error {
	ids := make([]string, len(people))
	for i, p := range people {
		pw, err := pwd(p)
		if err != nil {
			return err
		}
		id, err := s.person(ctx, p, pw)
		if err != nil {
			return fmt.Errorf("persona %s: %w", p.Email, err)
		}
		ids[i] = id
	}
	itemIDs := make([]string, len(seedItems))
	for i, it := range seedItems {
		id, err := s.item(ctx, ids[it.Owner], it, it.Title+suffix)
		if err != nil {
			return fmt.Errorf("objeto %q: %w", it.Title+suffix, err)
		}
		itemIDs[i] = id
	}
	for i, b := range seedBookings {
		owner := ids[seedItems[b.Item].Owner]
		if err := s.booking(ctx, b, itemIDs[b.Item], owner, ids[b.Requester]); err != nil {
			return fmt.Errorf("reserva %d: %w", i+1, err)
		}
	}
	return nil
}

func (s *seeder) admin(ctx context.Context, password string) error {
	email := authdomain.NormalizeEmail(seedAdminEmail)
	u, err := s.d.users.ByEmail(ctx, email)
	switch {
	case err == nil && u.Role == authdomain.RoleAdmin:
		s.people.existing++
		s.logf("admin    %s ya existe", email)
		return nil
	case err == nil:
		return fmt.Errorf("%s existe sin rol admin; no se modificó", email)
	case !authdomain.Is(err, authdomain.ErrUserNotFound):
		return fmt.Errorf("buscar %s: %w", email, err)
	}
	_, problems, err := newAdminAccount(ctx, s.d.admins, email, password, s.d.now())
	if len(problems) > 0 {
		return fmt.Errorf("admin %s: %s %s", email, problems[0].Field, problems[0].Issue)
	}
	if err != nil {
		return fmt.Errorf("crear admin %s: %w", email, err)
	}
	s.people.created++
	s.logf("admin    %s creado", email)
	return nil
}

func (s *seeder) person(ctx context.Context, p seedPerson, password string) (string, error) {
	email := authdomain.NormalizeEmail(p.Email)
	u, err := s.d.users.ByEmail(ctx, email)
	if err == nil {
		s.people.existing++
		s.logf("persona  %s ya existe (no se cambia su contraseña)", email)
		return u.ID, nil
	}
	if !authdomain.Is(err, authdomain.ErrUserNotFound) {
		return "", err
	}
	res, err := s.d.auth.Register(ctx, authusecase.RegisterInput{
		Email: email, Password: password, DisplayName: p.DisplayName, AcceptTerms: true, IsAdult: true,
	})
	if err != nil {
		return "", err
	}
	s.people.created++
	s.logf("persona  %s creada", email)
	return res.User.ID, nil
}

func (s *seeder) item(ctx context.Context, ownerID string, it seedItem, title string) (string, error) {
	viewer := itemsusecase.Viewer{ID: ownerID}
	if id, err := s.findItem(ctx, viewer, title); err != nil || id != "" {
		if id != "" {
			s.objects.existing++
			s.logf("objeto   %q ya existe", title)
		}
		return id, err
	}
	data, err := seedAssets.ReadFile("assets/" + it.Photo)
	if err != nil {
		return "", fmt.Errorf("foto %s: %w", it.Photo, err)
	}
	ref, err := s.d.photos.Upload(ctx, ownerID, data)
	if err != nil {
		return "", fmt.Errorf("subir foto: %w", err)
	}
	view, err := s.d.items.Create(ctx, viewer, itemsusecase.ItemInput{
		Title: title, Description: it.Description, Category: it.Category, Condition: it.Condition,
		Modality: it.Modality, DeclaredValueCents: it.DeclaredValueCents, PricePerDayCents: it.PricePerDayCents,
		PriceSource: itemsdomain.PriceManual, Zone: itemsdomain.Zone{Municipality: it.Municipality},
		Photos: []imagesdomain.Ref{ref},
	})
	if err != nil {
		// CA-09: la foto ya subida no queda huérfana en Cloudinary.
		if derr := s.d.photos.Destroy(ctx, ref); derr != nil {
			return "", errors.Join(err, fmt.Errorf("y no se pudo borrar la foto subida: %w", derr))
		}
		return "", err
	}
	s.objects.created++
	s.logf("objeto   %q creado", title)
	return view.Item.ID, nil
}

// findItem busca un objeto de la persona por título exacto (sin removidos).
func (s *seeder) findItem(ctx context.Context, viewer itemsusecase.Viewer, title string) (string, error) {
	q := itemsusecase.ListQuery{Limit: itemsusecase.MaxPageSize}
	for {
		page, err := s.d.items.ListMine(ctx, viewer, q)
		if err != nil {
			return "", err
		}
		for _, v := range page.Items {
			if v.Item.Title == title {
				return v.Item.ID, nil
			}
		}
		if page.Next == nil {
			return "", nil
		}
		q.After = page.Next
	}
}

func (s *seeder) booking(ctx context.Context, b seedBooking, itemID, ownerID, requesterID string) error {
	requester := bookingsusecase.Viewer{ID: requesterID}
	existing, err := s.findBooking(ctx, requester, itemID)
	if err != nil {
		return err
	}
	if existing != nil {
		s.reservations.existing++
		s.logf("reserva  del objeto %s ya existe (%s)", itemID, existing.Status)
		// Completa una ejecución interrumpida (creada pero sin su transición);
		// nunca cambia una reserva que ya avanzó a otro estado.
		if existing.Status == bookingsdomain.StatusPending && b.Final != seedPending {
			return s.transition(ctx, b.Final, existing.ID, ownerID, requesterID)
		}
		return nil
	}
	start := s.d.today().AddDays(b.StartInDays)
	in := bookingsusecase.QuoteInput{ItemID: itemID, Start: start.String(), End: start.AddDays(b.Days - 1).String(), PaymentMode: b.Payment}
	q, err := s.d.bookings.Quote(ctx, requester, in)
	if err != nil {
		return fmt.Errorf("cotizar: %w", err)
	}
	v, err := s.d.bookings.Create(ctx, requester, bookingsusecase.CreateInput{QuoteInput: in, AcceptedTotalCents: q.TotalCents})
	if err != nil {
		return fmt.Errorf("solicitar: %w", err)
	}
	s.reservations.created++
	s.logf("reserva  del objeto %s creada", itemID)
	return s.transition(ctx, b.Final, v.Booking.ID, ownerID, requesterID)
}

func (s *seeder) transition(ctx context.Context, final seedBookingState, id, ownerID, requesterID string) error {
	switch final {
	case seedAccepted:
		if _, err := s.d.bookings.Accept(ctx, bookingsusecase.Viewer{ID: ownerID}, id, nil); err != nil {
			return fmt.Errorf("aceptar: %w", err)
		}
	case seedCancelled:
		reason := "Datos de demostración"
		if _, err := s.d.bookings.Cancel(ctx, bookingsusecase.Viewer{ID: requesterID}, id, &reason); err != nil {
			return fmt.Errorf("cancelar: %w", err)
		}
	}
	return nil
}

// findBooking busca la reserva de requester sobre itemID en cualquier
// estado. Se identifica por objeto y solicitante, no por fechas: las fechas
// son relativas a hoy y cambiarían de un día a otro (plan P-13).
func (s *seeder) findBooking(ctx context.Context, requester bookingsusecase.Viewer, itemID string) (*bookingsdomain.Booking, error) {
	q := bookingsusecase.ListQuery{Role: bookingsdomain.RoleRequester, Limit: 50}
	for {
		page, err := s.d.bookings.List(ctx, requester, q)
		if err != nil {
			return nil, err
		}
		for _, v := range page.Items {
			if v.Booking.ItemID == itemID {
				b := v.Booking
				return &b, nil
			}
		}
		if page.Next == nil {
			return nil, nil
		}
		q.After = page.Next
	}
}

// randomPassword genera una contraseña de 32 caracteres para las copias de
// --scale. No se guarda ni se muestra.
func randomPassword() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
