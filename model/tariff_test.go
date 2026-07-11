package model

import (
	"testing"

	"gopkg.in/guregu/null.v4"
)

func zvtTariff() Tariff {
	return Tariff{
		Type:                  VZP,
		UseTimeTariff:         true,
		TimeTariff1Active:     true,
		TimeTariff1Name:       null.StringFrom("Tag"),
		TimeTariff1From:       null.StringFrom("06:00"),
		TimeTariff1To:         null.StringFrom("08:00"),
		TimeTariff1CentPerKWh: null.FloatFrom(22.5),
		TimeTariff2Active:     true,
		TimeTariff2Name:       null.StringFrom("Nacht"),
		TimeTariff2From:       null.StringFrom("20:00"),
		TimeTariff2To:         null.StringFrom("06:00"),
		TimeTariff2CentPerKWh: null.FloatFrom(5.5),
	}
}

func TestValidateTimeTariff(t *testing.T) {
	t.Run("valid incl. midnight crossing", func(t *testing.T) {
		tariff := zvtTariff()
		if err := tariff.ValidateTimeTariff(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("simple tariff skips validation", func(t *testing.T) {
		tariff := Tariff{Type: VZP, UseTimeTariff: false}
		if err := tariff.ValidateTimeTariff(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("EEG tariff rejected", func(t *testing.T) {
		tariff := zvtTariff()
		tariff.Type = EEG
		if err := tariff.ValidateTimeTariff(); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("freeKWh rejected in time mode", func(t *testing.T) {
		tariff := zvtTariff()
		tariff.FreeKWh = null.IntFrom(100)
		if err := tariff.ValidateTimeTariff(); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("from == to rejected", func(t *testing.T) {
		tariff := zvtTariff()
		tariff.TimeTariff1To = null.StringFrom("06:00")
		if err := tariff.ValidateTimeTariff(); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("off-raster time rejected", func(t *testing.T) {
		tariff := zvtTariff()
		tariff.TimeTariff1From = null.StringFrom("06:10")
		if err := tariff.ValidateTimeTariff(); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("missing price rejected", func(t *testing.T) {
		tariff := zvtTariff()
		tariff.TimeTariff1CentPerKWh = null.Float{}
		if err := tariff.ValidateTimeTariff(); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("overlap rejected", func(t *testing.T) {
		tariff := zvtTariff()
		// 05:00-07:00 ueberlappt das Mitternachtsfenster 20:00-06:00
		tariff.TimeTariff1From = null.StringFrom("05:00")
		tariff.TimeTariff1To = null.StringFrom("07:00")
		if err := tariff.ValidateTimeTariff(); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("overlap contained window rejected", func(t *testing.T) {
		tariff := zvtTariff()
		tariff.TimeTariff1From = null.StringFrom("21:00")
		tariff.TimeTariff1To = null.StringFrom("23:00")
		if err := tariff.ValidateTimeTariff(); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("adjacent windows allowed", func(t *testing.T) {
		tariff := zvtTariff()
		// 06:00-08:00 grenzt exakt an 20:00-06:00 (Bis exklusiv) - erlaubt
		if err := tariff.ValidateTimeTariff(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("single active window valid", func(t *testing.T) {
		tariff := zvtTariff()
		tariff.TimeTariff2Active = false
		tariff.TimeTariff2From = null.String{}
		tariff.TimeTariff2To = null.String{}
		tariff.TimeTariff2CentPerKWh = null.Float{}
		if err := tariff.ValidateTimeTariff(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}
