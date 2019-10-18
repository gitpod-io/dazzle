package fancylog

import (
	"github.com/gookit/color"
	"github.com/sirupsen/logrus"
)

// Formatter formats log output
type Formatter struct {
	Level int
}

// DefaultIndent is the spacing for any output
const DefaultIndent = "              "

// Format renders a single log entry
func (f *Formatter) Format(entry *logrus.Entry) ([]byte, error) {
	var res []byte
	for i := 0; i < f.Level; i++ {
		res = append(res, []byte(color.BgWhite.Sprint("  "))...)
	}

	step, ok := entry.Data["step"]
	if ok {
		res = append(res, []byte(color.Sprintf("<fg=black;bg=white> step %02d </>", step))...)
		res = append(res, ' ')
	} else {
		res = append(res, []byte("          ")...)
	}

	emoji, ok := entry.Data["emoji"]
	if ok {
		res = append(res, []byte(emoji.(string))...)
		res = append(res, []byte("  ")...)
	} else {
		res = append(res, []byte("    ")...)
	}

	var cl *color.Theme
	switch entry.Level {
	case logrus.DebugLevel:
		cl = color.Debug
	case logrus.InfoLevel:
		cl = color.Info
	case logrus.WarnLevel:
		cl = &color.Theme{Name: "warning", Style: color.Style{color.Yellow}}
	case logrus.ErrorLevel:
		cl = color.Error
	case logrus.FatalLevel:
		cl = color.Danger
	}
	res = append(res, []byte(cl.Sprintf("%-44s", entry.Message))...)

	for k, v := range entry.Data {
		if k == "step" || k == "emoji" {
			continue
		}

		res = append(res, []byte(color.FgDarkGray.Sprintf("%s=\"%s\" ", k, v))...)
	}

	res = append(res, '\n')
	return res, nil
}

// Push increases the level by one
func (f *Formatter) Push() {
	f.Level++
}

// Pop decreases the level by one
func (f *Formatter) Pop() {
	f.Level--
}
