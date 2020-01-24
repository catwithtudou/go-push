package gateway

import (
	"sync"
	"github.com/owenliang/go-push/common"
)

// 房间
type Room struct {
	rwMutex sync.RWMutex
	roomId string
	id2Conn map[uint64]*WSConnection
}

func InitRoom(roomId string) (room *Room) {
	room = &Room {
		roomId: roomId,
		id2Conn: make(map[uint64]*WSConnection),
	}
	return
}

//将连接加入房间
func (room *Room) Join(wsConn *WSConnection) (err error) {
	var (
		existed bool
	)

	room.rwMutex.Lock()
	defer room.rwMutex.Unlock()

	//判断是否加入房间,或者房间是否存在,并保证一个连接只能加入一个房间或者一个桶
	if _, existed = room.id2Conn[wsConn.connId]; existed {
		err = common.ERR_JOIN_ROOM_TWICE
		return
	}

	room.id2Conn[wsConn.connId] = wsConn
	return
}

//退出房间
func (room *Room) Leave(wsConn* WSConnection) (err error) {
	var (
		existed bool
	)

	room.rwMutex.Lock()
	defer room.rwMutex.Unlock()

	if _, existed = room.id2Conn[wsConn.connId]; !existed {
		err = common.ERR_NOT_IN_ROOM
		return
	}

	//将此连接从room的字典中删除
	delete(room.id2Conn, wsConn.connId)
	return
}

//房间的连接字典中长度
func (room *Room) Count() int {
	room.rwMutex.RLock()
	defer room.rwMutex.RUnlock()

	return len(room.id2Conn)
}

//向房间内用户推送消息
func (room *Room) Push(wsMsg *common.WSMessage) {
	var (
		wsConn *WSConnection
	)
	room.rwMutex.RLock()
	defer room.rwMutex.RUnlock()

	//全量非堵塞推送
	for _, wsConn = range room.id2Conn {
		wsConn.SendMessage(wsMsg)
	}
}