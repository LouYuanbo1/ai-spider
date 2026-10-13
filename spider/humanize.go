package spider

import (
	"math/rand/v2"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/input"
	"github.com/go-rod/rod/lib/proto"
)

// Humanize 复用 Spider.humanize 字段，由 config.yaml 的 rod.humanize 控制。
// 开启时：打字逐字+随机停顿、偶发打错、点击前 hover 停顿、分段式滚动；
// 关闭时：退化为原有的一次性输入/单次点击/瞬时滚动。

// 打字节奏参数（毫秒）
const (
	typeDelayMinMs = 40    // 单字符最小间隔
	typeDelayMaxMs = 220   // 单字符最大间隔
	typoProb       = 0.025 // 每个字符打错的概率
)

// 滚动节奏参数（毫秒）：每一段滚动后的"阅读"停留时间范围
const (
	scrollPauseMinMs = 500
	scrollPauseMaxMs = 1800
)

// clickDelayMinMs / clickDelayMaxMs：点击前在目标上停留的随机时长（毫秒）
const (
	clickDelayMinMs = 200
	clickDelayMaxMs = 700
)

// humanDelay 在 [min,max] 毫秒之间随机休眠，模拟人类操作间隔。
// 仅当 humanize 开启时才会真正等待；关闭时立即返回。
func (s *Spider) humanDelay(minMs, maxMs int) {
	if !s.humanize {
		return
	}
	if maxMs <= minMs {
		time.Sleep(time.Duration(minMs) * time.Millisecond)
		return
	}
	ms := rand.IntN(maxMs-minMs) + minMs
	time.Sleep(time.Duration(ms) * time.Millisecond)
}

// randChance 以概率 p 返回 true，用于决定是否触发一次"打错"。
func randChance(p float64) bool {
	return rand.Float64() < p
}

// nearestTypoKey 在目标字符附近随机取一个"容易按错的"字母当作输入事故。
// 这里用一个常见的字母池近似，避免依赖具体键盘布局；runes 会先排除原字符。
func nearestTypoKey(r rune) input.Key {
	pool := []rune("abcdefghijklmnopqrstuvwxyz")
	candidate := pool[rand.IntN(len(pool))]
	if candidate == r {
		// 换个候选，避免"打错"和原字符相同而看不出效果
		candidate = pool[(rand.IntN(len(pool))+13)%len(pool)]
	}
	return input.Key(candidate)
}

// typeLikeHuman 分两种方式模拟真人输入：
//   - 可打印键盘字符（ASCII 0x20~0x7E）：用底层按键事件逐字派发，字符间随机停顿，偶发打错后退格重打。
//   - 非键盘字符（中文/emoji/CJK 等）：无法用按键事件派发（CDP 只有真实键盘按键），
//     累积后改用 Input.insertText 整批插入，批间同样随机停顿，以逼近真人输入节奏。
//
// clearFlag 为 true 时先清空目标原有内容（全选再覆盖）。
func (s *Spider) typeLikeHuman(el *rod.Element, text string, clearFlag bool) error {
	if clearFlag {
		if err := el.SelectAllText(); err != nil {
			return err
		}
		s.humanDelay(120, 400)
	}

	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}

	// pending 累积连续的非键盘字符，整批插入
	var pending strings.Builder
	flush := func() error {
		if pending.Len() == 0 {
			return nil
		}
		if err := el.Input(pending.String()); err != nil {
			return err
		}
		pending.Reset()
		s.humanDelay(80, 250)
		return nil
	}

	for _, r := range text {
		// 仅可打印键盘字符才走逐字按键；其余（中文等）进入待插入缓冲
		if r >= 0x20 && r <= 0x7E {
			if err := flush(); err != nil {
				return err
			}
			// 偶发打错：先输入一个错误字符，稍作停顿后退格删掉再继续
			if randChance(typoProb) {
				if err := el.Type(nearestTypoKey(r)); err != nil {
					return err
				}
				s.humanDelay(80, 220)
				if err := el.Type(input.Backspace); err != nil {
					return err
				}
				s.humanDelay(80, 220)
			}

			if err := el.Type(input.Key(r)); err != nil {
				return err
			}
			s.humanDelay(typeDelayMinMs, typeDelayMaxMs)
		} else {
			pending.WriteRune(r)
		}
	}
	return flush()
}

// clickLikeHuman 模拟真人点击：先把目标滚到可视区并悬停，停留随机时长后再点击。
// 返回后由调用方决定是否等待网络空闲。
func (s *Spider) clickLikeHuman(el *rod.Element) error {
	if err := el.ScrollIntoView(); err != nil {
		return err
	}
	if err := el.Hover(); err != nil {
		return err
	}
	s.humanDelay(clickDelayMinMs, clickDelayMaxMs)
	return el.Click(proto.InputMouseButtonLeft, 1)
}

// scrollLikeHuman 分段式滚动并穿插随机"阅读"停顿，模拟真人边读边往下翻。
// totalY 为总滚动距离（可为负，向上滚动）。
func (s *Spider) scrollLikeHuman(p *rod.Page, totalY int) error {
	if !s.humanize || totalY == 0 {
		_, err := p.Eval(`(dy) => window.scrollBy({ top: dy, behavior: 'smooth' })`, totalY)
		return err
	}

	direction := 1
	if totalY < 0 {
		direction = -1
	}
	remaining := totalY * direction

	// 随机拆成 2~4 段
	segments := rand.IntN(3) + 2
	for segments > 0 && remaining > 0 {
		segments--

		// 每段步长由剩余量打折再加抖动，避免严格的均分（显得机械）
		step := remaining / (segments + 1)
		if step <= 0 {
			step = 1
		}
		if randChance(0.5) {
			step = step + rand.IntN(step+1)/2 + 1
		}
		if step > remaining {
			step = remaining
		}

		if _, err := p.Eval(`(dy) => window.scrollBy({ top: dy, behavior: 'smooth' })`, step*direction); err != nil {
			return err
		}
		remaining -= step

		// 每段滚完后"阅读"一会儿（停留时长随机）
		s.humanDelay(scrollPauseMinMs, scrollPauseMaxMs)
	}
	return nil
}
