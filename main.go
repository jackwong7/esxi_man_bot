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
	"strings"

	"rs/esxi"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"
	"github.com/joho/godotenv"
)

var messageMap = make(map[int64]MessageInfo)

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
			// Check if update is a callback query from Inline Keyboard
			if update.CallbackQuery != nil {
				handleInlineKeyboard(update.CallbackQuery, bot)
			}
			continue
		}

		// 增加判断，如果用户发送的是 `/restart` 命令且是您指定的用户，则发送一条新的消息
		if update.Message.Command() == "restart" && update.Message.From.ID == 917774935 {
			// 发送一条新的消息
			msg := tgbotapi.NewMessage(update.Message.Chat.ID, "请在命令中指定要重启的虚拟机，例如：/restart vm_name")
			msg.ReplyToMessageID = update.Message.MessageID
			bot.Send(msg)
			continue
		}

		// 如果用户不是指定的用户，则记录消息信息并不做处理
		if update.Message.From.ID != 917774935 {
			messageInfo := MessageInfo{
				ChatID:     update.Message.Chat.ID,
				MessageID:  update.Message.MessageID,
				CreateTime: update.Message.Date,
			}
			messageMap[update.Message.Chat.ID] = messageInfo
			continue
		}

		if !update.Message.IsCommand() {
			go send("你输入的不是命令!", update, bot)
			continue
		}
		switch update.Message.Command() {
		case "start":
			msg := tgbotapi.NewMessage(update.Message.Chat.ID, "欢迎使用 ESXi Bot，您可以使用以下命令：\n/list - 获取当前 ESXi 主机上的虚拟机列表\n/restart - 重启指定虚拟机")
			bot.Send(msg)
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
			replyMsg := fmt.Sprintf("获取成功: \n%s", listStr)
			// Create Inline Keyboard
			inlineKeyboard := createInlineKeyboard(lists)
			if len(inlineKeyboard.InlineKeyboard) > 0 {
				replyMsg += "\n请选择要重启的虚拟机："
				inlineKeyboardMessage := tgbotapi.NewMessage(update.Message.Chat.ID, replyMsg)
				inlineKeyboardMessage.ReplyMarkup = &inlineKeyboard
				bot.Send(inlineKeyboardMessage)
			} else {
				go edit(m.Chat.ID, m.MessageID, replyMsg, bot)
			}
			break
		case "restart":
			args := update.Message.CommandArguments()
			if args == "" {
				go send("请输入要重启的虚拟机名称，例如：/restart vm_name", update, bot)
				break
			}
			m, _ := send(fmt.Sprintf("重启中, 请稍等..."), update, bot)
			err := restartVM(args)
			if err != nil {
				go edit(m.Chat.ID, m.MessageID, fmt.Sprintf("操作失败, 原因:%s", err.Error()), bot)
				break
			}
			go edit(m.Chat.ID, m.MessageID, fmt.Sprintf("%s 已重启", args), bot)
			break
		default:
			//...
		}
	}
}

type MessageInfo struct {
	ChatID     int64
	MessageID  int
	CreateTime int
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

func createInlineKeyboard(vmList []mo.VirtualMachine) tgbotapi.InlineKeyboardMarkup {
	var buttons []tgbotapi.InlineKeyboardButton
	var rows [][]tgbotapi.InlineKeyboardButton
	for i, vm := range vmList {
		buttonText := fmt.Sprintf("%s", vm.Config.Name)
		callbackData := fmt.Sprintf("restart_%s", vm.Config.Name)
		button := tgbotapi.NewInlineKeyboardButtonData(buttonText, callbackData)
		buttons = append(buttons, button)
		if (i+1)%5 == 0 || i == len(vmList)-1 {
			rows = append(rows, buttons)
			buttons = []tgbotapi.InlineKeyboardButton{}
		}
	}
	if len(rows) == 0 {
		return tgbotapi.InlineKeyboardMarkup{}
	}
	inlineKeyboard := tgbotapi.NewInlineKeyboardMarkup(rows...)
	return inlineKeyboard
}

func handleInlineKeyboard(callbackQuery *tgbotapi.CallbackQuery, bot *tgbotapi.BotAPI) {
	if strings.HasPrefix(callbackQuery.Data, "restart_") {
		vmName := strings.TrimPrefix(callbackQuery.Data, "restart_")
		err := restartVM(vmName)
		if err != nil {
			errMsg := fmt.Sprintf("操作失败, 原因:%s", err.Error())
			answerCallbackQuery := tgbotapi.NewCallback(callbackQuery.ID, errMsg)
			bot.AnswerCallbackQuery(answerCallbackQuery)
			go sendNewMessage(callbackQuery.Message.Chat.ID, errMsg, bot)
			return
		}
		replyMsg := fmt.Sprintf("%s 已重启", vmName)
		answerCallbackQuery := tgbotapi.NewCallback(callbackQuery.ID, replyMsg)
		bot.AnswerCallbackQuery(answerCallbackQuery)
		go sendNewMessage(callbackQuery.Message.Chat.ID, replyMsg, bot)
	}
}
func sendNewMessage(chatID int64, msg string, bot *tgbotapi.BotAPI) {
	m := tgbotapi.NewMessage(chatID, msg)
	bot.Send(m)
}
