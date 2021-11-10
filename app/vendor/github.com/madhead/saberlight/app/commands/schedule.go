package commands

import (
	"os"
	"strings"

	"github.com/madhead/saberlight/app/cli"
	"github.com/madhead/saberlight/app/util"
	"github.com/madhead/saberlight/app/util/log"

	"github.com/paypal/gatt"
)

// Schedule gets bulb's schedule
func Schedule() {
	util.Operate(func(device gatt.Device, peripheral gatt.Peripheral, done chan bool) {
		if strings.ToUpper(*cli.ScheduleTarget) == strings.ToUpper(peripheral.ID()) {
			device.Handle(gatt.PeripheralConnected(func(peripheral gatt.Peripheral, err error) {
				defer device.CancelConnection(peripheral)

				characteristic, err := util.GetCharacteristic(peripheral, gatt.MustParseUUID("FFD5"), gatt.MustParseUUID("FFD9"))

				if (err != nil) || (nil == characteristic) {
					log.Error.Printf("Failed to get characteristic: %v\n", err)
					os.Exit(util.ExitStatusGenericError)
				}

				listen, err := util.GetCharacteristicWithDescriptors(peripheral, gatt.MustParseUUID("FFD0"), gatt.MustParseUUID("FFD4"))

				if (err != nil) || (nil == listen) {
					log.Error.Printf("Failed to get listen characteristic: %v\n", err)
					os.Exit(util.ExitStatusGenericError)
				}

				chunks := make(chan []byte)
				var data []byte

				peripheral.SetNotifyValue(listen, func(characteristic *gatt.Characteristic, data []byte, err error) {
					chunks <- data

					// 0x52 indicated end of timings data
					if data[len(data)-1] == 0x52 {
						close(chunks)
					}
				})
				peripheral.WriteCharacteristic(characteristic, []byte{0x24, 0x2A, 0x2B, 0x42}, false)

				// Wait for the timings data
				for chunk := range chunks {
					data = append(data, chunk...)
				}
				for i := 0; i < 6; i++ {
					log.Info.Printf("Timing #%v: Days: %v, Hour: %02v, Minute: %02v, Turn on?: %v, Open?: %v\n", i+1, days(data[i*14+8]), data[i*14+5], data[i*14+6], data[i*14+14] == 240, data[i*14+1] == 240)
				}

				done <- true
			}))

			device.StopScanning()
			device.Connect(peripheral)
		}
	})
}

const (
	all = 254
	mon = 2
	tue = 4
	wed = 8
	thu = 16
	fri = 32
	sat = 64
	sun = 128
)

func days(days byte) []string {
	if days == all {
		return []string{"EVERY DAY"}
	}

	var result []string

	if days&mon != 0 {
		result = append(result, "MON")
	}
	if days&tue != 0 {
		result = append(result, "TUE")
	}
	if days&wed != 0 {
		result = append(result, "WED")
	}
	if days&thu != 0 {
		result = append(result, "THU")
	}
	if days&fri != 0 {
		result = append(result, "FRI")
	}
	if days&sat != 0 {
		result = append(result, "SAT")
	}
	if days&sun != 0 {
		result = append(result, "SUN")
	}

	return result
}
