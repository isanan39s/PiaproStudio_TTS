package main

// メッセージバスコントローラ
// 宛先を見て適切なchanにメッセージを転送する
// 入力1本、出力複数なスイッチ
// 最初から最後まで使うchanを登録することが推奨されます

type MsgBus struct {
	Cmd    string
	To     string
	From   string ///自称
	Option []string
}

type BusHQdat struct {
	addrTab map[string]chan MsgBus
}

// / 宛先とchanの関連付け
func (hq *BusHQdat) registAddr(addr string, toChan chan MsgBus) {
	hq.addrTab[addr] = toChan
}

// / 宛先とchanの解決
func (hq *BusHQdat) resolveAddr(addr string) chan MsgBus {
	return hq.addrTab[addr]
}

// / 受け取り
func BusHQ(msg chan MsgBus, endchan chan struct{}) *BusHQdat {
	BusHQdat := BusHQdat{
		addrTab: map[string]chan MsgBus{},
	}

	go func() {
		for {
			select {
			case msgA := <-msg:
				BusHQdat.sendMsg(msgA)
			case <-endchan:
				return
			}
		}
	}()
	return &BusHQdat
}

// / 送信
func (hq *BusHQdat) sendMsg(msg MsgBus) {
	toChan := hq.resolveAddr(msg.To)
	if toChan != nil {
		toChan <- msg
	}
}
