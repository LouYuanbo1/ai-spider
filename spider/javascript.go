package spider

import (
	"context"
	"fmt"
	"time"

	"github.com/go-rod/rod/lib/proto"
)

// doJavaScript 执行JS并回调处理结果
func (s *Spider) DoJavaScript(ctx context.Context, js string, args ...any) (string, error) {
	// 无参时用 async 函数表达式包裹（而非立即执行的 IIFE）：
	// rod 的 Eval 会以 .apply(this, arguments) 调用该函数表达式并 awaitPromise，
	// 若写成 IIFE 则会得到一个 Promise，再被 .apply 调用时报 "apply is not a function"。
	var source string
	if len(args) > 0 {
		source = js
	} else {
		source = "async () => { " + js + "\n}"
	}

	run := func() (*proto.RuntimeRemoteObject, error) {
		return s.page.Eval(source, args...)
	}

	result, err := run()
	if err != nil {
		// 模型生成的 JS 在 SPA 页面（如 B 站）上偶发失败，
		// 重试一次，避免一次偶发异常就中断整次抓取。
		s.logger.Warn("JS执行失败，重试中", "js", js, "err", err)
		time.Sleep(300 * time.Millisecond)
		result, err = run()
		if err != nil {
			s.logger.Error("JS执行失败", "js", js, "err", err)
			return "", fmt.Errorf("JS执行失败: %v", err)
		}
	}

	// 脚本未 return / 返回 undefined 时给一个明确结果，而不是后续序列化崩溃。
	if result == nil || result.Type == proto.RuntimeRemoteObjectTypeUndefined || result.Value.Nil() {
		return "undefined", nil
	}

	data, err := result.Value.MarshalJSON()
	if err != nil {
		return "", fmt.Errorf("JSON序列化失败: %v", err)
	}

	return string(data), nil
}
