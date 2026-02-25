package vm

type VMService struct {
    vmcount int
}

func NewVMService() *VMService {
    return &VMService{
        vmcount: 0,
    }
}

func (vm *VMService) CreateVM(roomID string) error {
    return nil
}

func (vm *VMService) DestroyVM(roomID string) error {
    return nil
}
