package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/LouYuanbo1/ai-spider/agent"
	"github.com/LouYuanbo1/ai-spider/config"
	"github.com/LouYuanbo1/ai-spider/spider"
	"github.com/cloudwego/eino-ext/components/model/deepseek"
)

func main() {
	appcfg, err := config.Load("config/config.yaml")
	if err != nil {
		log.Fatalf("解析配置失败: %v", err)
	}
	ctx := context.Background()

	chatModel, err := deepseek.NewChatModel(ctx, &deepseek.ChatModelConfig{
		APIKey: appcfg.Model.APIKey,
		Model:  "deepseek-flash",
	})
	if err != nil {
		fmt.Printf("Error creating chat model: %v", err)
		return
	}

	spider, err := spider.InitSpider(appcfg.Rod)
	if err != nil {
		fmt.Printf("Error creating spider: %v", err)
		return
	}

	modelAgent, err := agent.NewDefaultSpiderAgent(ctx, chatModel, spider)
	if err != nil {
		fmt.Printf("Error creating spider agent: %v", err)
		return
	}

	// 终端多轮对话模式：for 循环读取用户输入，逐轮调用 agent。
	// 暂时不保存历史记忆，每轮独立执行。
	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Print("\n>>> 请输入指令 (输入 exit/quit/q 退出): ")
		line, err := reader.ReadString('\n')
		if err != nil {
			// EOF (Ctrl+D / Ctrl+Z) 也视为退出
			fmt.Println("\n[输入结束，退出]")
			break
		}

		input := strings.TrimSpace(line)
		if input == "" {
			continue
		}
		switch strings.ToLower(input) {
		case "exit", "quit", "q":
			fmt.Println("[退出]")
			return
		}

		modelAgent.OutputMessage(ctx, input, func(s string) {
			fmt.Print(s)
		})
		fmt.Println()
	}
}
