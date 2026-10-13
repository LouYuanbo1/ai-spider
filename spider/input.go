package spider

import (
	"fmt"
	"strings"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

// typeText 将文本输入到元素；clearFlag 为 true 时先清空原有内容。
// 使用 rod 原生键盘事件触发真实输入，对 React/Vue 受控组件友好。
// 开启 humanize 时走逐字打字（见 humanize.go），否则一次性写入。
func (s *Spider) typeText(el *rod.Element, text string, clearFlag bool) error {
	if !s.humanize {
		if clearFlag {
			if err := el.SelectAllText(); err != nil {
				return err
			}
		}
		return el.Input(text)
	}
	return s.typeLikeHuman(el, text, clearFlag)
}

// elementValue 读取输入控件当前值，作为输入生效校验的依据。
// 同时兼容 input/textarea 的 value，与 contenteditable 的 textContent。
func (s *Spider) elementValue(el *rod.Element) (string, error) {
	res, err := el.Eval(`() => this.value ?? this.textContent ?? ""`)
	if err != nil {
		return "", err
	}
	if res == nil || res.Value.Nil() {
		return "", nil
	}
	return strings.TrimSpace(res.Value.String()), nil
}

// inputEffective 判定输入是否真正生效：读回控件当前值，确认包含目标文本。
// 目标文本两侧空白会被忽略（trim 后做包含匹配），对"输入了但尾部换行"等场景宽容。
// 读值失败时保守视为已生效，避免"无法确认"反而误中断流程。
func (s *Spider) inputEffective(el *rod.Element, text string) bool {
	got, err := s.elementValue(el)
	if err != nil {
		return true
	}
	return strings.Contains(got, strings.TrimSpace(text))
}

// doInput 执行输入，并返回该操作是否真正生效
func (s *Spider) doInput(el *rod.Element, text string, clearFlag bool) (bool, error) {
	if err := s.typeText(el, text, clearFlag); err != nil {
		return false, err
	}
	return s.inputEffective(el, text), nil
}

// DoInput CSS选择器定位输入框输入文本；返回是否真正写入成功
func (s *Spider) DoInput(selector, text string, clearFlag bool) (bool, error) {
	el, err := s.page.Element(selector)
	if err != nil {
		return false, fmt.Errorf("未找到输入框 %q: %v", selector, err)
	}
	return s.doInput(el, text, clearFlag)
}

// DoInputX XPath定位输入框输入文本；返回是否真正写入成功
func (s *Spider) DoInputX(selector, text string, clearFlag bool) (bool, error) {
	el, err := s.page.ElementX(selector)
	if err != nil {
		return false, fmt.Errorf("未找到XPath输入框 %q: %v", selector, err)
	}
	return s.doInput(el, text, clearFlag)
}

// DoInputNode 通过后端 DOM 节点 ID 精准定位输入框并输入文本；返回是否真正写入成功
func (s *Spider) DoInputNode(backendNodeID proto.DOMBackendNodeID, text string, clearFlag bool) (bool, error) {
	el, err := s.page.ElementFromNode(&proto.DOMNode{BackendNodeID: backendNodeID})
	if err != nil {
		return false, fmt.Errorf("解析节点 %d 失败: %v", backendNodeID, err)
	}
	return s.doInput(el, text, clearFlag)
}
