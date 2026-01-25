package dto

type MessageDTO struct {
	ID           string `json:"id"`
	IsMine       bool   `json:"is_mine"`
	SenderID     string `json:"sender_id"`
	TargetID     string `json:"target_id"`
	TargetPubKey string `json:"target_pub_key,omitempty"`
	Text         string `json:"text"`
	Timestamp    int64  `json:"timestamp"`
}

type PeerDTO struct {
	PeerID          string `json:"peer_id"`
	ShortID         string `json:"short_id"`
	Address         string `json:"addr,omitempty"`
	ProtocolVersion uint32 `json:"protocol_version"`
	IsConnected     bool   `json:"is_connected"`
	IsLocal         bool   `json:"is_local"`
	LastSeen        string `json:"last_seen"`
}

type FileDTO struct {
	Name string `json:"name,omitempty"`
	Size uint64 `json:"size,omitempty"`
	Path string `json:"path,omitempty"`
}

type LogDTO struct {
	Time    string `json:"time"`
	Level   string `json:"level"`
	Message string `json:"message"`
}

type ContactDTO struct {
	PeerID   string `json:"peer_id"`
	PubKey   string `json:"pub_key"`
	Alias    string `json:"alias"`
	Avatar   string `json:"avatar_base64"`
	IsOnline bool   `json:"is_online"`
	LastSeen int64  `json:"last_seen"`
}
