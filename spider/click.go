package spider

import (
	"fmt"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

// clickElement 对目标元素执行真人化点击，并在点击后等待网络空闲。
func (s *Spider) clickElement(el *rod.Element) error {
	if err := s.clickLikeHuman(el); err != nil {
		return err
	}
	s.page.WaitRequestIdle(500*time.Millisecond, nil, nil, nil)
	return nil
}

// DoClick CSS选择器点击，点击后等待网络空闲
func (s *Spider) DoClick(selector string) error {
	el, err := s.page.Element(selector)
	if err != nil {
		return fmt.Errorf("未找到元素 %q: %v", selector, err)
	}
	return s.clickElement(el)
}

// DoClickX XPath点击，点击后等待网络空闲
func (s *Spider) DoClickX(selector string) error {
	el, err := s.page.ElementX(selector)
	if err != nil {
		return fmt.Errorf("未找到XPath元素 %q: %v", selector, err)
	}
	return s.clickElement(el)
}

// DoClickNode 通过后端 DOM 节点 ID 精准点击，避免同名元素/选择器歧义
func (s *Spider) DoClickNode(backendNodeID proto.DOMBackendNodeID) error {
	el, err := s.page.ElementFromNode(&proto.DOMNode{BackendNodeID: backendNodeID})
	if err != nil {
		return fmt.Errorf("解析节点 %d 失败: %v", backendNodeID, err)
	}
	return s.clickElement(el)
}
