package main

import (
	"embed"

	bookingsdomain "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/bookings/domain"
	itemsdomain "github.com/VaivenEnterprise/VaivenApplication-Backend/internal/items/domain"
)

// seedAssets son las fotos de ejemplo del seed (≤ 100 KB cada una, generadas
// por programa, sin datos de personas ni contenido de terceros).
//
//go:embed assets/*.jpg
var seedAssets embed.FS

// seedPerson es una persona de demostración. La contraseña NO está aquí: sale
// del archivo de SEED_CREDENTIALS_FILE (spec 012, P-12).
type seedPerson struct {
	Email       string
	DisplayName string
}

// seedAdminEmail es la cuenta de administración de la demostración.
const seedAdminEmail = "admin.demo@vaiven.example"

// seedPeople son las 6 personas de la demostración (dominio .example: no
// existen correos reales).
var seedPeople = []seedPerson{
	{"ana.demo@vaiven.example", "Ana Demo"},
	{"beto.demo@vaiven.example", "Beto Demo"},
	{"carla.demo@vaiven.example", "Carla Demo"},
	{"diego.demo@vaiven.example", "Diego Demo"},
	{"elena.demo@vaiven.example", "Elena Demo"},
	{"fer.demo@vaiven.example", "Fer Demo"},
}

// seedItem es un objeto de demostración; Owner es el índice en seedPeople.
type seedItem struct {
	Owner              int
	Title              string
	Description        string
	Category           string
	Condition          itemsdomain.Condition
	Modality           itemsdomain.Modality
	DeclaredValueCents int64
	PricePerDayCents   int64
	Municipality       string
	Photo              string // archivo dentro de assets/
}

// seedItems son los 15 objetos (dinero en centavos de MXN, Art. V).
var seedItems = []seedItem{
	{0, "Taladro percutor 600 W", "Incluye estuche y brocas para concreto y madera.", "herramientas_electricas", itemsdomain.ConditionGood, itemsdomain.ModalityRent, 120000, 8000, "Guadalajara", "objeto-01.jpg"},
	{0, "Escalera de aluminio 2 m", "Ligera, con seguro de apertura.", "construccion_jardineria", itemsdomain.ConditionLikeNew, itemsdomain.ModalityRent, 150000, 6000, "Guadalajara", "objeto-02.jpg"},
	{0, "Juego de desarmadores", "12 piezas, puntas planas y de cruz.", "herramientas_manuales", itemsdomain.ConditionGood, itemsdomain.ModalityLoan, 30000, 0, "Guadalajara", "objeto-03.jpg"},
	{1, "Bicicleta de montaña rodada 26", "Frenos de disco, casco incluido.", "movilidad", itemsdomain.ConditionGood, itemsdomain.ModalityRent, 450000, 15000, "Zapopan", "objeto-04.jpg"},
	{1, "Tienda de campaña 4 personas", "Impermeable, fácil de armar.", "deportes", itemsdomain.ConditionLikeNew, itemsdomain.ModalityRent, 200000, 9000, "Zapopan", "objeto-05.jpg"},
	{1, "Podadora manual", "Para pasto corto, sin motor.", "construccion_jardineria", itemsdomain.ConditionFair, itemsdomain.ModalityLoan, 80000, 0, "Zapopan", "objeto-06.jpg"},
	{2, "Hidrolavadora 1800 PSI", "Manguera de 5 m y dos boquillas.", "herramientas_electricas", itemsdomain.ConditionGood, itemsdomain.ModalityRent, 300000, 12000, "Tlaquepaque", "objeto-07.jpg"},
	{2, "Mesa plegable", "1.80 m, para 8 personas.", "hogar", itemsdomain.ConditionGood, itemsdomain.ModalityRent, 90000, 5000, "Tlaquepaque", "objeto-08.jpg"},
	{2, "Sierra de mano", "Hoja de 20 pulgadas.", "herramientas_manuales", itemsdomain.ConditionWorn, itemsdomain.ModalityLoan, 25000, 0, "Tlaquepaque", "objeto-09.jpg"},
	{3, "Patines en línea talla 26", "Con protecciones de rodilla y codo.", "deportes", itemsdomain.ConditionGood, itemsdomain.ModalityRent, 110000, 4000, "Guadalajara", "objeto-10.jpg"},
	{3, "Carretilla", "Llanta neumática, capacidad 80 L.", "construccion_jardineria", itemsdomain.ConditionFair, itemsdomain.ModalityRent, 130000, 5000, "Guadalajara", "objeto-11.jpg"},
	{4, "Licuadora industrial", "Vaso de acero de 4 L.", "hogar", itemsdomain.ConditionLikeNew, itemsdomain.ModalityRent, 250000, 10000, "Zapopan", "objeto-12.jpg"},
	{4, "Scooter eléctrico", "Autonomía de 20 km, cargador incluido.", "movilidad", itemsdomain.ConditionGood, itemsdomain.ModalityRent, 800000, 25000, "Zapopan", "objeto-13.jpg"},
	{5, "Caja de herramientas básica", "Martillo, pinzas, llaves y cinta.", "herramientas_manuales", itemsdomain.ConditionGood, itemsdomain.ModalityLoan, 60000, 0, "Tonalá", "objeto-14.jpg"},
	{5, "Proyector portátil", "HDMI, ideal para reuniones.", "otros", itemsdomain.ConditionGood, itemsdomain.ModalityRent, 350000, 14000, "Tonalá", "objeto-15.jpg"},
}

// seedBookingState es el estado final de una reserva de demostración.
type seedBookingState string

const (
	seedPending   seedBookingState = "pending"
	seedAccepted  seedBookingState = "accepted"
	seedCancelled seedBookingState = "cancelled"
)

// seedBooking es una reserva de demostración. Item y Requester son índices;
// las fechas son días desde hoy (en el futuro: todo pasa por los casos de
// uso, sin escribir fechas del pasado a mano).
type seedBooking struct {
	Item, Requester int
	StartInDays     int
	Days            int
	Payment         bookingsdomain.PaymentMode
	Final           seedBookingState
}

// seedBookings son las 3 reservas en estados distintos.
var seedBookings = []seedBooking{
	{Item: 0, Requester: 1, StartInDays: 10, Days: 2, Payment: bookingsdomain.PaymentFullOnDelivery, Final: seedPending},
	{Item: 3, Requester: 2, StartInDays: 14, Days: 3, Payment: bookingsdomain.PaymentSplit5050, Final: seedAccepted},
	{Item: 2, Requester: 3, StartInDays: 20, Days: 1, Payment: bookingsdomain.PaymentNone, Final: seedCancelled},
}
