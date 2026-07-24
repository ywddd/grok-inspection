package main

import (
	"fmt"
	"strings"
)

func renderUIPage(pluginID string) []byte {
	base := "/v0/management/plugins/" + pluginID
	var b strings.Builder
	b.Grow(len(uiDocHead) + len(uiCSS) + len(uiDocMid) + len(uiI18NJS) + len(uiScriptLang) +
		len(uiScriptCore) + len(uiScriptInspect) + len(uiScriptSchedule) + len(uiScriptTable) +
		len(uiScriptPoll) + len(uiScriptWire) + len(uiScriptBan) + len(uiDocTail) + len(base) + 64)
	b.WriteString(uiDocHead)
	b.WriteString(uiCSS)
	b.WriteString(uiDocMid)
	b.WriteString(uiI18NJS)
	b.WriteString(uiScriptLang)
	b.WriteString(fmt.Sprintf("  const BASE = %q;\n", base))
	b.WriteString(uiScriptCore)
	b.WriteString(uiScriptInspect)
	b.WriteString(uiScriptSchedule)
	b.WriteString(uiScriptTable)
	b.WriteString(uiScriptPoll)
	b.WriteString(uiScriptWire)
	b.WriteString(uiScriptBan)
	b.WriteString(uiDocTail)
	return []byte(b.String())
}
