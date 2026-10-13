package spider

import "time"

// DoScroll 页面滚动，滚动后等待网络空闲以捕获懒加载内容。
// 开启 humanize 时分段滚动并在每段间随机"阅读"停留（见 humanize.go）。
func (s *Spider) DoScroll(scrollY int) error {
	if err := s.scrollLikeHuman(s.page, scrollY); err != nil {
		return err
	}
	s.page.WaitRequestIdle(500*time.Millisecond, nil, nil, nil)
	return nil
}
