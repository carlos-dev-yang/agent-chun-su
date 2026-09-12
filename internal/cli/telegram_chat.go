package cli

import (
	"chunsu/internal/config"
	"chunsu/internal/errorreport"
	"chunsu/internal/telegram"
	"chunsu/internal/telegramchat"
	"context"
	"github.com/spf13/cobra"
)

func serveTelegram(cmd *cobra.Command, root string, c config.Config, binding telegram.Binding, client *telegram.Client) error {
	ctx, cancel := context.WithCancel(cmd.Context())
	reports := errorreport.New(root, c.Limits)
	done := make(chan struct{})
	go func() { defer close(done); maintainChatHost(ctx, cmd, root, c, reports) }()
	defer func() { cancel(); <-done }()
	receiver := telegramchat.Receiver{Root: root, Config: c, Binding: binding, Client: client, Errors: reports}
	return receiver.Serve(ctx)
}
