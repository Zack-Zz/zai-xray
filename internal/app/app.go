package app

import (
	"fmt"
	"os"
	"strings"

	"github.com/zhouze/zai-xray/internal/config"
	"github.com/zhouze/zai-xray/internal/providers"
	"github.com/zhouze/zai-xray/internal/render"
	"github.com/zhouze/zai-xray/internal/store"
)

type OutputFlags struct {
	JSON    bool
	OneLine bool
	Pretty  bool
	NoColor bool
}

type App struct {
	Config   config.Config
	Store    *store.SQLiteStore
	Runner   *providers.OpenAIProvider
	Render   *render.Renderer
	OutMode  render.Mode
	UseColor bool
}

func New(configPath string, flags OutputFlags) (*App, error) {
	cfg, err := config.Load(configPath)
	if err != nil {
		return nil, err
	}
	st, err := store.NewSQLite(cfg.DBPath)
	if err != nil {
		return nil, err
	}

	mode := render.ModePretty
	switch {
	case flags.JSON:
		mode = render.ModeJSON
	case flags.OneLine:
		mode = render.ModeOneLine
	case flags.Pretty:
		mode = render.ModePretty
	}
	useColor := render.DefaultUseColor(flags.NoColor)
	r := render.New(os.Stdout, render.Options{Mode: mode, UseColor: useColor})

	openai := providers.NewOpenAIProvider(cfg.OpenAI.APIKey, cfg.OpenAI.BaseURL, cfg.TimeoutDuration())
	app := &App{
		Config:   cfg,
		Store:    st,
		Runner:   openai,
		Render:   r,
		OutMode:  mode,
		UseColor: useColor,
	}
	if err := st.Migrate(nilContext()); err != nil {
		_ = st.Close()
		return nil, fmt.Errorf("migrate db: %w", err)
	}
	return app, nil
}

func (a *App) Close() error {
	if a.Store != nil {
		return a.Store.Close()
	}
	return nil
}

func EffectiveProvider(flagProvider string, cfg config.Config) string {
	if strings.TrimSpace(flagProvider) != "" {
		return flagProvider
	}
	return cfg.DefaultProvider
}

func EffectiveModel(flagModel string, cfg config.Config) string {
	if strings.TrimSpace(flagModel) != "" {
		return flagModel
	}
	return cfg.DefaultModel
}
