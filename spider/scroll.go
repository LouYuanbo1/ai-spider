package spider

import (
	"context"
)

// DoScroll 页面滚动，滚动后等待网络空闲以捕获懒加载内容。
// 开启 humanize 时分段滚动并在每段间随机"阅读"停留（见 humanize.go）。
func (s *Spider) DoScroll(ctx context.Context, scrollY int) error {
	p := s.page.Context(ctx)
	if err := s.scrollLikeHuman(p, scrollY); err != nil {
		return err
	}
	return nil
}
