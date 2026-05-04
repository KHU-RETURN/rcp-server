package ssh

func Init(notifySockPath string, notifySecret []byte) *Service {
	return NewService(NewNotifyClient(notifySockPath, notifySecret))
}
