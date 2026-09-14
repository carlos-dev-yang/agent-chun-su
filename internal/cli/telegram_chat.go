package cli

import (
	"chunsu/internal/config"
	"chunsu/internal/telegram"
	"chunsu/internal/telegramchat"
	"github.com/spf13/cobra"
)

func serveTelegram(cmd *cobra.Command, root string, c config.Config, binding telegram.Binding, client *telegram.Client) error {
	receiver := telegramchat.Receiver{Root: root, Config: c, Binding: binding, Client: client}
	return receiver.Serve(cmd.Context())
}
