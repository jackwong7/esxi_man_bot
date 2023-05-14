package main

import (
	"context"
	"fmt"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"
	"log"
	"net/http"
	"net/url"
	"os"

	"rs/esxi"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"
	"github.com/joho/godotenv"
)

func main() {
	// Load environment variables from .env file
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	// Set up HTTP proxy if provided
	if proxyURL := os.Getenv("HTTP_PROXY"); proxyURL != "" {
		log.Printf("Using HTTP proxy: %s", proxyURL)
		proxy, _ := url.Parse(proxyURL)
		http.DefaultTransport = &http.Transport{
			Proxy: http.ProxyURL(proxy),
		}
	}

	// Create new bot instance
	bot, err := tgbotapi.NewBotAPI(os.Getenv("BOT_TOKEN"))
	if err != nil {
		log.Fatal(err)
	}

	// Set up updates channel
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates, err := bot.GetUpdatesChan(u)
	if err != nil {
		log.Fatal(err)
	}

	// Handle incoming updates
	for update := range updates {
		if update.Message == nil {
			continue
		}
		if update.Message.From.ID != 917774935 {
			go send("操作失败!", update, bot)
			continue
		}

		if !update.Message.IsCommand() {
			go send("你输入的不是命令!", update, bot)
			continue
		}
		switch update.Message.Command() {
		case "list":
			m, _ := send(fmt.Sprintf("获取中, 请稍等..."), update, bot)
			lists, err := getVMLists()
			if err != nil {
				go edit(m.Chat.ID, m.MessageID, fmt.Sprintf("操作失败, 原因:%s", err.Error()), bot)
				break
			}
			var listStr string
			for _, list := range lists {
				var state string
				if list.Summary.Runtime.PowerState == types.VirtualMachinePowerStatePoweredOn {
					state = "正常✅"
				} else if list.Summary.Runtime.PowerState == types.VirtualMachinePowerStatePoweredOff {
					state = "关机⛔️"
				} else {
					state = "暂停⛔️"
				}
				listStr += fmt.Sprintf("%-12s %s\n", list.Config.Name, state)
			}
			go edit(m.Chat.ID, m.MessageID, fmt.Sprintf("获取成功: \n%s", listStr), bot)
			break
		case "restart":
			m, _ := send(fmt.Sprintf("重启中, 请稍等..."), update, bot)
			err := restartVM(update.Message.CommandArguments())
			if err != nil {
				go edit(m.Chat.ID, m.MessageID, fmt.Sprintf("操作失败, 原因:%s", err.Error()), bot)
				break
			}
			go edit(m.Chat.ID, m.MessageID, fmt.Sprintf("操作成功"), bot)
		default:
			//...
		}
	}
}

func send(msg string, update tgbotapi.Update, bot *tgbotapi.BotAPI) (tgbotapi.Message, error) {
	m := tgbotapi.NewMessage(update.Message.Chat.ID, msg)
	m.ReplyToMessageID = update.Message.MessageID
	return bot.Send(m)
}
func edit(chatID int64, messageID int, msg string, bot *tgbotapi.BotAPI) {
	m := tgbotapi.NewEditMessageText(chatID, messageID, msg)
	bot.Send(m)
}

func getVMLists() ([]mo.VirtualMachine, error) {
	ctx := context.Background()
	c, err := esxi.NewClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("login esxi failed, reason:%s", err.Error())
	}
	defer c.Logout(ctx)
	vm_lists, err := esxi.ListVirtualMachines(ctx, c)
	if err != nil {
		return nil, fmt.Errorf("failed to get vm lists, reason:%s", err.Error())
	}
	return vm_lists, nil
}

func restartVM(name string) error {
	ctx := context.Background()
	c, err := esxi.NewClient(ctx)
	if err != nil {
		return fmt.Errorf("login esxi failed, reason:%s", err.Error())
	}
	defer c.Logout(ctx)
	err = esxi.RebootVirtualMachine(context.Background(), c, name)
	if err != nil {
		return err
	}
	return nil
}
