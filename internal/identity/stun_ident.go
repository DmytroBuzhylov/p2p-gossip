package identity

import (
	"fmt"

	"github.com/pion/stun"
)

func StunIdent() (string, error) {
	client, err := stun.Dial("udp", "stun.l.google.com:19302")
	if err != nil {
		return "", fmt.Errorf("Не удалось подключиться к STUN серверу: %v\n", err)
	}
	defer client.Close()

	message := stun.MustBuild(stun.TransactionID, stun.BindingRequest)

	done := make(chan struct{})

	var xorAddr stun.XORMappedAddress
	err = client.Do(message, func(res stun.Event) {
		defer close(done)

		if res.Error != nil {
			fmt.Printf("Ошибка STUN запроса: %v\n", res.Error)
			return
		}

		if err := xorAddr.GetFrom(res.Message); err != nil {
			fmt.Printf("Не удалось получить адрес из сообщения: %v\n", err)
			return
		}
	})

	if err != nil {
		return "", fmt.Errorf("Ошибка при попытке выполнить Do: %v\n", err)
	}

	<-done

	return xorAddr.String(), nil
}
