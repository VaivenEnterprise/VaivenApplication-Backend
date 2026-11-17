package config

import "strings"

// Proveedores de IA aceptados en AI_PROVIDER (spec 014, RF-03 y T016).
// openai es cualquier API compatible con OpenAI (Groq en staging); ollama usa
// el mismo adaptador en local; gemini queda como alternativa.
const (
	AIProviderOpenAI = "openai"
	AIProviderGemini = "gemini"
	AIProviderOllama = "ollama"
)

// Valores por defecto de la IA (spec 014; plan P-01 y P-05).
const (
	DefaultOpenAIBaseURL = "https://api.groq.com/openai/v1"
	DefaultOpenAIModel   = "openai/gpt-oss-120b" // texto; Groq no ofrece hoy un modelo con visión
	DefaultGeminiBaseURL = "https://generativelanguage.googleapis.com"
	DefaultOllamaBaseURL = "http://localhost:11434/v1"
	DefaultGeminiModel   = "gemini-2.5-flash"
	maxAssistantRadiusKm = 50
)

// AI es la configuración de la IA generativa (spec 014): sugerencias al
// publicar (parte A) y asistente de proyectos (parte B).
type AI struct {
	// SuggestionsEnabled activa la parte A (AI_ENABLED). Apagada por defecto.
	SuggestionsEnabled bool
	// AssistantEnabled activa la parte B (ASSISTANT_ENABLED). Apagada por defecto.
	AssistantEnabled bool
	// Provider es openai (Groq u otro compatible), gemini u ollama (solo development).
	Provider string
	// BaseURL es la raíz de la API del proveedor (AI_BASE_URL).
	BaseURL string
	// Model es el modelo del proveedor (AI_MODEL).
	Model string
	// APIKey es la clave del proveedor (AI_API_KEY); nunca se imprime.
	APIKey Secret
	// DailyLimit son las sugerencias por persona y día (AI_DAILY_LIMIT, RF-05).
	DailyLimit int
	// AssistantDailyLimit son los mensajes al asistente por persona y día
	// (ASSISTANT_DAILY_LIMIT, RF-15).
	AssistantDailyLimit int
	// AssistantRadiusKm es el radio de búsqueda de coincidencias
	// (ASSISTANT_RADIUS_KM, RF-11).
	AssistantRadiusKm int
}

// Enabled indica si alguna de las dos funciones está encendida.
func (a AI) Enabled() bool { return a.SuggestionsEnabled || a.AssistantEnabled }

// loadAI lee las variables AI_* y ASSISTANT_*. Las dos funciones nacen
// apagadas; encender alguna con Gemini exige AI_API_KEY. Ollama solo se acepta
// en development (no cabe en la VM; spec 014, RF-03) y, en staging y
// production, el proveedor debe hablarse por HTTPS (Art. VI.6).
func loadAI(r *reader, appEnv string) AI {
	a := AI{
		SuggestionsEnabled:  r.boolean("AI_ENABLED", false),
		AssistantEnabled:    r.boolean("ASSISTANT_ENABLED", false),
		Provider:            r.oneOf("AI_PROVIDER", AIProviderOpenAI, AIProviderOpenAI, AIProviderGemini, AIProviderOllama),
		APIKey:              Secret(r.text("AI_API_KEY", "")),
		DailyLimit:          r.positiveInt("AI_DAILY_LIMIT", 20),
		AssistantDailyLimit: r.positiveInt("ASSISTANT_DAILY_LIMIT", 30),
		AssistantRadiusKm:   r.positiveInt("ASSISTANT_RADIUS_KM", 10),
	}
	defURL, defModel := DefaultOpenAIBaseURL, DefaultOpenAIModel
	switch a.Provider {
	case AIProviderGemini:
		defURL, defModel = DefaultGeminiBaseURL, DefaultGeminiModel
	case AIProviderOllama:
		defURL, defModel = DefaultOllamaBaseURL, ""
	}
	a.BaseURL = strings.TrimRight(r.text("AI_BASE_URL", defURL), "/")
	a.Model = r.text("AI_MODEL", defModel)

	if a.AssistantRadiusKm > maxAssistantRadiusKm {
		r.fail("ASSISTANT_RADIUS_KM", "debe ser como máximo 50")
	}
	if !strings.HasPrefix(a.BaseURL, "https://") && !strings.HasPrefix(a.BaseURL, "http://") {
		r.fail("AI_BASE_URL", "debe empezar con https:// o http://")
	}
	if isDeployed(appEnv) && !strings.HasPrefix(a.BaseURL, "https://") {
		r.fail("AI_BASE_URL", "en staging y production debe usar https://")
	}
	if a.Provider == AIProviderOllama && appEnv != EnvDevelopment {
		r.fail("AI_PROVIDER", "ollama solo se permite en development")
	}
	if a.Enabled() {
		if a.Model == "" {
			r.fail("AI_MODEL", "obligatoria cuando AI_ENABLED o ASSISTANT_ENABLED está activo")
		}
		if a.Provider != AIProviderOllama && a.APIKey == "" {
			r.fail("AI_API_KEY", "obligatoria con "+a.Provider+" cuando AI_ENABLED o ASSISTANT_ENABLED está activo")
		}
	}
	return a
}
