package main

import (
	"log"

	"fyne.io/systray"
)

func runTray(localURL, settingsURL, viewURL string, getTailscaleURL func() string, onQuit func()) {
	systray.Run(func() {
		systray.SetIcon(iconICO)
		systray.SetTitle("Scout")
		systray.SetTooltip("Moorebot Scout Control")

		mOpen := systray.AddMenuItem("Open Control", "Local UI (best for piloting)")
		mTS := systray.AddMenuItem("Open Tailscale HTTPS", "Remote / phone over Tailnet")
		mSettings := systray.AddMenuItem("Settings", "Drive base")
		mView := systray.AddMenuItem("Pop-out View", "Second monitor video")
		systray.AddSeparator()
		mQuit := systray.AddMenuItem("Quit", "Stop control center")

		go func() {
			for {
				select {
				case <-mOpen.ClickedCh:
					openURL(localURL)
				case <-mTS.ClickedCh:
					if u := getTailscaleURL(); u != "" {
						openURL(u + "/")
					} else {
						log.Printf("tailscale serve URL not available")
					}
				case <-mSettings.ClickedCh:
					openURL(settingsURL)
				case <-mView.ClickedCh:
					openURL(viewURL)
				case <-mQuit.ClickedCh:
					systray.Quit()
					return
				}
			}
		}()
	}, onQuit)
}
