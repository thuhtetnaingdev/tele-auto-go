package contextbuilder

import (
	"fmt"
	"strings"

	"tele-auto-go/internal/behavior"
	"tele-auto-go/internal/util"
)

type MessageLine struct {
	Direction string
	Text      string
}

const baseSystemPrompt = "You are replying as a real human Telegram user. " +
	"Write one short natural Telegram reply in plain text, or at most two short messages if behavior constraints explicitly request a split reply. " +
	"Match the language and tone of the latest incoming message and recent conversation. " +
	"Stay conservative and natural: not flirty, not promotional, not assistant-like. " +
	"Keep it brief, ideally under 40 words unless context strongly requires more. " +
	"No markdown, no labels, no explanation, no quotes. " +
	"Do not mention being AI. Return only the final reply text."

func Build(chatWith string, recent []MessageLine, latestIncoming string, soulPrompt string, constraints behavior.Constraints) (systemPrompt, userPrompt string) {
	if strings.TrimSpace(chatWith) == "" {
		chatWith = "unknown"
	}

	var lines []string
	for i, m := range recent {
		role := "other_person"
		if m.Direction == "me" {
			role = "me"
		}
		lines = append(lines, fmt.Sprintf("%d. [%s] %s", i+1, role, safeText(m.Text)))
	}

	userPrompt = strings.Join([]string{
		"Chat with: " + chatWith,
		"",
		"Recent conversation:",
		strings.Join(lines, "\n"),
		"",
		"Latest incoming message:",
		safeText(latestIncoming),
		"",
		buildBehaviorPrompt(constraints),
		"",
		"Instruction:",
		"Write one short natural reply for Telegram in the same language and tone as the conversation.",
	}, "\n")

	systemPrompt = baseSystemPrompt
	soulPrompt = strings.TrimSpace(soulPrompt)
	if soulPrompt != "" {
		if len(soulPrompt) > 4000 {
			soulPrompt = soulPrompt[:4000]
		}
		systemPrompt = strings.Join([]string{
			baseSystemPrompt,
			"",
			"Personality and reply-style profile (SOUL.md):",
			soulPrompt,
			"",
			"Use SOUL only if it does not conflict with hard rules above.",
		}, "\n")
	}

	return systemPrompt, userPrompt
}

func safeText(s string) string {
	return util.NormalizeSpace(s)
}

func buildBehaviorPrompt(constraints behavior.Constraints) string {
	text := strings.TrimSpace(behavior.BuildInstructionText(constraints))
	if text == "" {
		return "Behavior constraints: keep the reply natural and concise."
	}
	return "Behavior constraints:\n" + text
}
