package config

import (
	"net/url"
	"strings"
	"time"
)

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

// Tiempo de espera y razonamiento de las sugerencias (spec 014, T018).
const (
	DefaultSuggestTimeout = 25 * time.Second
	MinSuggestTimeout     = 5 * time.Second
	MaxSuggestTimeout     = 60 * time.Second

	// SuggestReasoningDefault no envía nada al proveedor; SuggestReasoningOff
	// desactiva el razonamiento (campo "reasoning" de OpenRouter).
	SuggestReasoningDefault = "default"
	SuggestReasoningOff     = "off"

	openRouterHost = "openrouter.ai"
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
	// Suggest es el proveedor de la parte A (SUGGEST_*; spec 014, T017). Cada
	// valor que falte toma el de AI_*, así que sin SUGGEST_* las dos partes
	// usan el mismo proveedor.
	Suggest Provider
	// SuggestTimeout es el tiempo máximo de la llamada al proveedor de las
	// sugerencias (SUGGEST_TIMEOUT; entre 5 s y 60 s; 25 s por defecto).
	SuggestTimeout time.Duration
	// SuggestReasoning es default u off (SUGGEST_REASONING). Sin definirla,
	// vale off si SUGGEST_BASE_URL es de openrouter.ai y default en otro caso.
	// Solo afecta a las sugerencias, nunca al asistente.
	SuggestReasoning string
}

// Provider es un proveedor de IA y su modelo.
type Provider struct {
	Name    string // openai, gemini u ollama
	BaseURL string
	Model   string
	APIKey  Secret
}

// Enabled indica si alguna de las dos funciones está encendida.
func (a AI) Enabled() bool { return a.SuggestionsEnabled || a.AssistantEnabled }

// Assistant devuelve el proveedor de la parte B (AI_*).
func (a AI) Assistant() Provider {
	return Provider{Name: a.Provider, BaseURL: a.BaseURL, Model: a.Model, APIKey: a.APIKey}
}

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
	a.Suggest = Provider{
		Name:    r.oneOf("SUGGEST_PROVIDER", a.Provider, AIProviderOpenAI, AIProviderGemini, AIProviderOllama),
		BaseURL: strings.TrimRight(r.text("SUGGEST_BASE_URL", a.BaseURL), "/"),
		Model:   r.text("SUGGEST_MODEL", a.Model),
		APIKey:  Secret(r.text("SUGGEST_API_KEY", a.APIKey.Reveal())),
	}
	a.SuggestTimeout = r.duration("SUGGEST_TIMEOUT", DefaultSuggestTimeout)
	if a.SuggestTimeout < MinSuggestTimeout || a.SuggestTimeout > MaxSuggestTimeout {
		r.fail("SUGGEST_TIMEOUT", "debe estar entre 5s y 60s")
		a.SuggestTimeout = DefaultSuggestTimeout
	}
	a.SuggestReasoning = r.oneOf("SUGGEST_REASONING", defaultSuggestReasoning(a.Suggest.BaseURL),
		SuggestReasoningDefault, SuggestReasoningOff)
	checkProvider(r, appEnv, "SUGGEST", a.Suggest, a.SuggestionsEnabled, "AI_ENABLED")
	// Al heredar de AI_*, el error debe nombrar la variable que falta de verdad.
	r.renameUnset("SUGGEST_MODEL", "AI_MODEL")
	r.renameUnset("SUGGEST_API_KEY", "AI_API_KEY")
	// AI_* es el proveedor del asistente; si solo están encendidas las
	// sugerencias y no hay SUGGEST_*, AI_* también es el suyo (ya revisado).
	checkProvider(r, appEnv, "AI", a.Assistant(), a.AssistantEnabled, "ASSISTANT_ENABLED")
	return a
}

// defaultSuggestReasoning apaga el razonamiento solo con OpenRouter, donde los
// modelos que razonan pueden gastar el presupuesto y devolver contenido vacío.
func defaultSuggestReasoning(baseURL string) string {
	if u, err := url.Parse(baseURL); err == nil && strings.EqualFold(u.Hostname(), openRouterHost) {
		return SuggestReasoningOff
	}
	return SuggestReasoningDefault
}

// checkProvider exige modelo y clave (salvo ollama) a un proveedor encendido.
func checkProvider(r *reader, appEnv, prefix string, p Provider, enabled bool, flag string) {
	if prefix == "SUGGEST" {
		if !strings.HasPrefix(p.BaseURL, "https://") && !strings.HasPrefix(p.BaseURL, "http://") {
			r.fail("SUGGEST_BASE_URL", "debe empezar con https:// o http://")
		}
		if isDeployed(appEnv) && !strings.HasPrefix(p.BaseURL, "https://") {
			r.fail("SUGGEST_BASE_URL", "en staging y production debe usar https://")
		}
		if p.Name == AIProviderOllama && appEnv != EnvDevelopment {
			r.fail("SUGGEST_PROVIDER", "ollama solo se permite en development")
		}
	}
	if !enabled {
		return
	}
	if p.Model == "" {
		r.fail(prefix+"_MODEL", "obligatoria cuando "+flag+" está activo")
	}
	if p.Name != AIProviderOllama && p.APIKey == "" {
		r.fail(prefix+"_API_KEY", "obligatoria con "+p.Name+" cuando "+flag+" está activo")
	}
}

// renameUnset cambia el nombre de la variable en los problemas ya anotados
// cuando la persona no definió key (se heredó de fallback).
func (r *reader) renameUnset(key, fallback string) {
	if _, ok := r.value(key); ok {
		return
	}
	for i, p := range r.problems {
		if strings.HasPrefix(p, key+":") {
			r.problems[i] = fallback + strings.TrimPrefix(p, key)
		}
	}
}
