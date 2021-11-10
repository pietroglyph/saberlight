package util

import (
	"errors"
	"os"
	"strings"
	"time"

	"github.com/madhead/saberlight/app/cli"
	"github.com/madhead/saberlight/app/util/log"
	"github.com/paypal/gatt"
)

type deviceResult struct {
	device gatt.Device
	err    error
}

// OpenHCI opens HCI device for future interactions
func OpenHCI() (gatt.Device, error) {
	// Sometimes gatt.NewDevice blocks indefinitely:
	// h, err := linux.NewHCI(d.devID, d.chkLE, d.maxConn)
	// h.resetDevice()
	// if err := h.c.SendAndCheckResp(s, []byte{0x00}); err != nil {
	// ...due to device doesn't respond to a command (e.g. opReset = hostCtl<<10 | 0x0003 // Reset)
	// so wrap this in gorutine and timeout it
	deviceReady := make(chan *deviceResult)

	// TODO: when running webserver, this gouroutine can run forever, causing leaks!
	go func() {
		device, err := gatt.NewDevice([]gatt.Option{
			gatt.LnxDeviceID(-1, true),
			gatt.LnxMaxConnections(1),
		}...)

		if err != nil {
			log.Error.Printf("Failed to open device: %v\n", err)
			deviceReady <- &deviceResult{
				device: nil,
				err:    err,
			}
			return
		}

		device.Init(func(device gatt.Device, state gatt.State) {
			if state == gatt.StatePoweredOn {
				deviceReady <- &deviceResult{
					device: device,
					err:    nil,
				}
			}
		})
	}()

	select {
	case <-time.After(*cli.DeviceTimeout):
		log.Error.Println("HCI device timed out")
		return nil, errors.New("HCI device timed out")
	case device := <-deviceReady:
		return device.device, device.err
	}
}

// GetCharacteristic searches for a characteristic by its UUID
func GetCharacteristic(peripheral gatt.Peripheral, serviceUUID gatt.UUID, characteristicUUID gatt.UUID) (*gatt.Characteristic, error) {
	return getCharacteristic(peripheral, serviceUUID, characteristicUUID, false)
}

// GetCharacteristicWithDescriptors searches for a characteristic by its UUID and also discovers descriptors for it, which is used when subscribing to notofications
func GetCharacteristicWithDescriptors(peripheral gatt.Peripheral, serviceUUID gatt.UUID, characteristicUUID gatt.UUID) (*gatt.Characteristic, error) {
	return getCharacteristic(peripheral, serviceUUID, characteristicUUID, true)
}

// Operate operates on discovered peripheral
func Operate(peripheralCallback func(device gatt.Device, peripheral gatt.Peripheral, done chan bool)) {
	device, err := OpenHCI()

	if err != nil {
		os.Exit(ExitStatusHCIError)
	}

	done := make(chan bool)

	device.Handle(gatt.PeripheralDiscovered(func(peripheral gatt.Peripheral, advertisement *gatt.Advertisement, rssi int) {
		peripheralCallback(device, peripheral, done)
	}))

	device.Scan([]gatt.UUID{}, false)

	select {
	case <-time.After(*cli.OperationTimeout):
		os.Exit(ExitStatusTimeout)
	case <-done:
	}
}

// Write writes some bytes into a characteristic
func Write(target string, serviceUUID string, characteristicUUID string, payload []byte) {
	Operate(func(device gatt.Device, peripheral gatt.Peripheral, done chan bool) {
		if strings.ToUpper(target) == strings.ToUpper(peripheral.ID()) {
			device.Handle(gatt.PeripheralConnected(func(peripheral gatt.Peripheral, err error) {
				defer device.CancelConnection(peripheral)

				characteristic, err := GetCharacteristic(peripheral, gatt.MustParseUUID(serviceUUID), gatt.MustParseUUID(characteristicUUID))

				if (err != nil) || (nil == characteristic) {
					os.Exit(ExitStatusGenericError)
				}

				peripheral.WriteCharacteristic(characteristic, payload, false)

				done <- true
			}))

			device.StopScanning()
			device.Connect(peripheral)
		}
	})
}

func getCharacteristic(peripheral gatt.Peripheral, serviceUUID gatt.UUID, characteristicUUID gatt.UUID, discoverDescriptors bool) (*gatt.Characteristic, error) {
	services, err := peripheral.DiscoverServices(nil)

	if err != nil {
		log.Error.Printf("Failed to discover services: %v\n", err)
		return nil, errors.New("Failed to discover services")
	}

	for _, service := range services {
		if service.UUID().Equal(serviceUUID) {
			characteristics, err := peripheral.DiscoverCharacteristics(nil, service)

			if err != nil {
				log.Error.Printf("Failed to discover characteristics: %v\n", err)
				return nil, errors.New("Failed to discover characteristics")
			}

			for _, characteristic := range characteristics {
				if characteristic.UUID().Equal(characteristicUUID) {
					if discoverDescriptors {
						_, err := peripheral.DiscoverDescriptors(nil, characteristic)

						if err != nil {
							log.Info.Printf("Failed to discover descriptors: %v\n", err)
						}
					}

					return characteristic, nil
				}
			}
		}
	}

	return nil, nil
}
