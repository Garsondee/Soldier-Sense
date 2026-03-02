package game

import "github.com/atotto/clipboard"

func setClipboardText(text string) error {
	return clipboard.WriteAll(text)
}
