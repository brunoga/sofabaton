# irfile test data

`uc-hex-vectors.json` is upstream's golden vectors for Unfolded Circle hex
codes (github.com/m3tac0de/home-assistant-sofabaton-x1s,
`tests/fixtures/uc-hex-vectors.json`, MIT licence): each code with the
timings Home Assistant's infrared-protocols library renders for it. The
Onkyo row is a real Remote Two export; the others are synthetic.

`denon-chart.txt` is five codes of Denon's IR Command Chart (summer 2016,
assets.denon.com/documentmaster/us/denon_ir_commandchart_summer_2016.xls),
in its Pronto form: Denon's own renderings of its Denon-K and 15-bit
codes, which irgen's must match.
