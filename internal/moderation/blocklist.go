package moderation

// Categorías de contenido prohibido (POL-16).
const (
	CategorySexual  = "sexual"
	CategoryWeapons = "armas"
	CategoryDrugs   = "drogas"
	CategoryIllegal = "ilegal"
)

// categoryLabel es cómo se nombra la categoría en el motivo que ve la persona.
var categoryLabel = map[string]string{
	CategorySexual:  "contenido sexual",
	CategoryWeapons: "armas",
	CategoryDrugs:   "drogas",
	CategoryIllegal: "actividades ilegales",
}

// blocklist son los términos y frases prohibidos por categoría (plan P-06,
// POL-16). Es una lista corta y revisable por el equipo: se escriben en
// minúsculas, con o sin acentos (se normalizan al cargar) y se buscan por
// palabra completa, así que "xxx" no bloquea "xxxl". Una frase se escribe con
// espacios. Se evitan palabras que también nombran herramientas u objetos
// comunes ("granada", "cristal", "pistola de calor").
var blocklist = map[string][]string{
	CategorySexual: {
		"porno", "pornografia", "pornografico", "pornografica", "xxx", "escort", "escorts",
		"prostitucion", "prostituta", "prostitutas", "nudes", "desnudos",
		"servicios sexuales", "sexo explícito", "contenido para adultos",
	},
	CategoryWeapons: {
		"pistola", "pistolas", "revólver", "rifle", "rifles", "fusil", "fusiles", "metralleta",
		"ak 47", "ar 15", "ak47", "ar15", "munición", "municiones", "explosivos", "dinamita",
		"bomba molotov", "arma de fuego", "armas de fuego", "silenciador para arma",
	},
	CategoryDrugs: {
		"droga", "drogas", "marihuana", "mariguana", "cannabis", "cocaína", "heroína",
		"metanfetamina", "metanfetaminas", "fentanilo", "lsd", "mdma", "éxtasis", "opio",
		"narcóticos", "hongos alucinógenos",
	},
	CategoryIllegal: {
		"mercancía robada", "objeto robado", "objetos robados", "producto robado", "productos robados",
		"artículo robado", "artículos robados", "documentos falsos", "documento falso",
		"identificación falsa", "licencia falsa", "pasaporte falso", "dinero falso", "billetes falsos",
		"tarjeta clonada", "tarjetas clonadas", "contrabando", "lavado de dinero",
		"tráfico de personas", "software pirata",
	},
}

// exceptions son frases legítimas que contienen un término de la lista (p. ej.
// herramientas que se prestan): se ignoran antes de buscar.
var exceptions = []string{
	"pistola de calor", "pistola de silicón", "pistola de silicon", "pistola de pintura",
	"pistola de impacto", "pistola de clavos", "pistola de grapas", "pistola neumática",
	"pistola engrapadora", "pistola de agua", "pistola de juguete", "pistola de gravedad",
}
