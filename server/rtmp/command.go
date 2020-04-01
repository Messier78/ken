package rtmp

import "ken/server/amf"

type Command struct {
	IsFlex        bool
	Name          string
	TransactionID uint32
	Objects       []interface{}
}

func (cmd *Command) Write(w Writer) (err error) {
	if cmd.IsFlex {
		if err = w.WriteByte(0x00); err != nil {
			return
		}
	}
	if _, err = amf.WriteString(w, cmd.Name); err != nil {
		return
	}
	if _, err = amf.WriteDouble(w, float64(cmd.TransactionID)); err != nil {
		return
	}
	for _, obj := range cmd.Objects {
		if _, err = amf.WriteValue(w, obj); err != nil {
			return
		}
	}
	return
}

func (cmd *Command) Dump() {
	logger.Debugf("Command{IsFlex: %t, Name: %s, TransactionID: %d, Objects: %+v}",
		cmd.IsFlex, cmd.Name, cmd.TransactionID, cmd.Objects)
}
