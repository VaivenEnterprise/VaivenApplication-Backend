package domain

// ReasonCode es el motivo de una recomendación; la app arma el texto en es-MX
// (RF-05; Art. V.4).
type ReasonCode string

// Motivos (RF-05).
const (
	ReasonSimilarTo         ReasonCode = "similar_to"
	ReasonPopularInCategory ReasonCode = "popular_in_category"
	ReasonPopularNearby     ReasonCode = "popular_nearby"
	ReasonPopular           ReasonCode = "popular"
)

// Reason explica una recomendación. ItemID e ItemTitle solo van con
// ReasonSimilarTo: el objeto visto o solicitado que más contribuyó.
type Reason struct {
	Code      ReasonCode
	ItemID    string
	ItemTitle string
}

// SimilarTo crea el motivo «parecido a».
func SimilarTo(itemID, itemTitle string) Reason {
	return Reason{Code: ReasonSimilarTo, ItemID: itemID, ItemTitle: itemTitle}
}
