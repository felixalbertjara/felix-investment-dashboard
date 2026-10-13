package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
)

func telegramConfigurationStatus() (bool, string) {
	token := strings.TrimSpace(os.Getenv("TELEGRAM_BOT_TOKEN"))
	chatID := strings.TrimSpace(os.Getenv("TELEGRAM_CHAT_ID"))
	if token == "" || chatID == "" {
		return false, "faltan TELEGRAM_BOT_TOKEN / TELEGRAM_CHAT_ID en GitHub Secrets"
	}
	return true, "credenciales cargadas desde GitHub Secrets"
}

func setupTelegramSecretsInteractive() error {
	return fmt.Errorf("en V3 Serverless Telegram se configura como GitHub Secrets; no se guarda en archivos")
}

func telegramSecretsPath() string { return "GitHub Secrets" }

func sendTelegramMessage(message string) error {
	token := strings.TrimSpace(os.Getenv("TELEGRAM_BOT_TOKEN"))
	chatID := strings.TrimSpace(os.Getenv("TELEGRAM_CHAT_ID"))
	if token == "" || chatID == "" {
		return fmt.Errorf("Telegram no configurado en GitHub Secrets")
	}
	endpoint := "https://api.telegram.org/bot" + token + "/sendMessage"
	form := url.Values{}
	form.Set("chat_id", chatID)
	form.Set("text", message)
	form.Set("disable_web_page_preview", "true")

	req, err := http.NewRequest(http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", "FelixInvestmentDashboardServerless/3.0")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Telegram HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var result struct {
		OK          bool   `json:"ok"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal(body, &result); err == nil && !result.OK {
		return fmt.Errorf("Telegram rechazó el mensaje: %s", result.Description)
	}
	return nil
}

func sendTestNotification() error {
	return sendTelegramMessage("✅ Felix Investment Dashboard V3 Serverless\n\nAlerta de prueba OK. GitHub + Telegram están funcionando.")
}

func notifyInvestmentAlert(p PortfolioView, why, popupMessage string) error {
	icon := "🟡"
	switch p.Alert {
	case "STOP - TESIS INVALIDADA", "STOP DINÁMICO - ASEGURAR", "SALIR / VENDER MUCHO", "PROTEGER CAPITAL":
		icon = "🚨"
	case "TOMAR GANANCIA FUERTE":
		icon = "🟠"
	case "TOMAR GANANCIA PARCIAL", "ROTAR CAPITAL / TIME STOP":
		icon = "🟡"
	case "PROTEGER GANANCIA":
		icon = "🔔"
	}
	msg := fmt.Sprintf("%s FELIX TRADING ALERT\n\n%s · %s\nCambio: %s\nPrecio: %s\nRendimiento: %.2f%%\nR: %.2fR\nVelocidad: %s\nRuedas: %d/%d (%s)\nCalendario: %d días\nExit score: %d\nVenta sugerida: %.0f%%\nStop inicial: %s\nStop efectivo: %s\nMáximo desde compra: %s\n\nAcción: %s\n\nMotivo: %s",
		icon, strings.ToUpper(p.Position.Ticker), p.Alert, why, money(p.Result.Quote.Price), p.PnLPct,
		p.RMultiple, p.VelocityLabel, p.HoldingSessions, p.ExpectedDays, p.Strategy, p.HoldingDays,
		p.ExitScore, p.SuggestedSellPct, money(p.InitialStopPrice), money(p.EffectiveStopPrice),
		money(p.MaxPriceSinceBuy), p.ExitAction, firstReason(p.ExitReasons))
	return sendTelegramMessage(msg)
}
