package ops

import (
	"github.com/innaka-tech/ai-toolkit/v2/internal/doc"
	"github.com/innaka-tech/ai-toolkit/v2/internal/task"
)

func sectionOf(t *task.Task, name string) (string, bool) { return doc.Section(t.Body, name) }

func setSection(t *task.Task, name, content string) string {
	return doc.SetSection(t.Body, name, content)
}
