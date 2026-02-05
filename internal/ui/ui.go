package ui

import (
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"gioui.org/app"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"example.com/ip-opt-gui/internal/domain"
	"example.com/ip-opt-gui/internal/engine"
	"example.com/ip-opt-gui/internal/filedialog"
	"example.com/ip-opt-gui/internal/hostsfile"
	"example.com/ip-opt-gui/internal/model"
)

type row struct {
	Domain  string
	BestIP  string
	Via     string
	Rate    float64
	P95     time.Duration
	Jitter  time.Duration
	Message string
	Apply   widget.Bool
}

type msgLog struct{ Line string }
type msgResult struct{ Result model.DomainResult }
type msgProgress struct{ Done, Total int }
type msgDone struct{ Err error }
type msgPickedPath struct {
	Kind string
	Path string
	Err  error
}

const (
	uiPad         unit.Dp = 12
	uiGap         unit.Dp = 10
	uiRadius      unit.Dp = 12
	uiRadiusSmall unit.Dp = 10
	uiBorder      unit.Dp = 1
	uiCtrlH       unit.Dp = 40
	uiCtrlHM      unit.Dp = 32
)

var (
	uiBg        = color.NRGBA{A: 255, R: 246, G: 247, B: 249}
	uiSurface   = color.NRGBA{A: 255, R: 255, G: 255, B: 255}
	uiBorderCol = color.NRGBA{A: 255, R: 224, G: 226, B: 230}
	uiText      = color.NRGBA{A: 255, R: 38, G: 38, B: 38}
	uiMuted     = color.NRGBA{A: 255, R: 110, G: 115, B: 125}
	uiPrimary   = color.NRGBA{A: 255, R: 47, G: 108, B: 246}
	uiDanger    = color.NRGBA{A: 255, R: 230, G: 70, B: 70}
)

func Run() {
	go func() {
		w := new(app.Window)
		w.Option(
			app.Title("IP 优选（hosts）"),
			app.Size(unit.Dp(980), unit.Dp(680)),
		)
		if err := loop(w); err != nil {
			os.Exit(1)
		}
		os.Exit(0)
	}()
	app.Main()
}

func loop(w *app.Window) error {
	th := material.NewTheme()
	th.TextSize = unit.Sp(14)
	th.FingerSize = uiCtrlH
	th.Palette = material.Palette{
		Bg:         uiBg,
		Fg:         uiText,
		ContrastBg: uiPrimary,
		ContrastFg: color.NRGBA{A: 255, R: 255, G: 255, B: 255},
	}

	var (
		domainsEd widget.Editor
		dnsEd     widget.Editor
		hostsEd   widget.Editor

		portEd        widget.Editor
		timeoutEd     widget.Editor
		attemptsEd    widget.Editor
		concurrencyEd widget.Editor

		ipv4 widget.Bool
		ipv6 widget.Bool

		startBtn   widget.Clickable
		stopBtn    widget.Clickable
		loadHosts  widget.Clickable
		pickFile   widget.Clickable
		previewBtn widget.Clickable
		writeBtn   widget.Clickable
		restoreBtn widget.Clickable
		pickHosts  widget.Clickable

		leftList    layout.List
		resultsList layout.List

		mainTab widget.Enum

		tabConfigBtn  widget.Clickable
		tabResultsBtn widget.Clickable
		tabLogBtn     widget.Clickable
		tabPreviewBtn widget.Clickable

		selectAllBtn  widget.Clickable
		selectNoneBtn widget.Clickable
		selectOKBtn   widget.Clickable

		logEd     widget.Editor
		previewEd widget.Editor

		rows      []row
		domainIdx = map[string]int{}

		logLines   []string
		previewTxt string

		running    bool
		lastBackup string

		domainFilePath string

		done, total int
		cancel      context.CancelFunc
	)

	domainsEd.SetText("")
	domainsEd.SingleLine = false
	dnsEd.SingleLine = false
	dnsEd.SetText(strings.Join([]string{
		"223.5.5.5",
		"114.114.114.114",
		"1.1.1.1",
		"8.8.8.8",
	}, "\n"))
	hostsEd.SingleLine = true
	hostsEd.SetText(hostsfile.DefaultHostsPath())

	portEd.SingleLine = true
	portEd.SetText("443")
	timeoutEd.SingleLine = true
	timeoutEd.SetText("1200")
	attemptsEd.SingleLine = true
	attemptsEd.SetText("3")
	concurrencyEd.SingleLine = true
	concurrencyEd.SetText("16")

	ipv4.Value = true
	ipv6.Value = false

	mainTab.Value = "config"
	logEd.SingleLine = false
	logEd.ReadOnly = true
	previewEd.SingleLine = false
	previewEd.ReadOnly = true

	leftList.Axis = layout.Vertical
	resultsList.Axis = layout.Vertical

	appendLog := func(s string) {
		if strings.TrimSpace(s) == "" {
			return
		}
		ts := time.Now().Format("15:04:05")
		logLines = append(logLines, fmt.Sprintf("[%s] %s", ts, s))
		if len(logLines) > 500 {
			logLines = logLines[len(logLines)-500:]
		}
		logEd.SetText(strings.Join(logLines, "\n"))
	}

	buildMappings := func() []hostsfile.Mapping {
		var ms []hostsfile.Mapping
		for _, r := range rows {
			if !r.Apply.Value || r.Domain == "" || r.BestIP == "" || r.Message != "" {
				continue
			}
			ms = append(ms, hostsfile.Mapping{IP: r.BestIP, Domain: r.Domain})
		}
		return ms
	}

	applyResult := func(res model.DomainResult) {
		if _, ok := domainIdx[res.Domain]; !ok {
			domainIdx[res.Domain] = len(rows)
			var r row
			r.Domain = res.Domain
			rows = append(rows, r)
		}
		i := domainIdx[res.Domain]
		r := rows[i]
		if res.Err != nil {
			r.Message = res.Err.Error()
			r.BestIP = ""
			r.Via = ""
			r.Rate = 0
			r.P95 = 0
			r.Jitter = 0
			r.Apply.Value = false
		} else {
			r.Message = ""
			r.BestIP = res.Best.IP.String()
			r.Via = res.Best.ResolvedVia
			r.Rate = res.Best.SuccessRate()
			r.P95 = res.Best.P95
			r.Jitter = res.Best.JitterStd
			r.Apply.Value = true
		}
		rows[i] = r
	}

	uiCh := make(chan any, 256)

	startRun := func() {
		domains := domain.ParseDomains(domainsEd.Text())
		if len(domains) == 0 {
			appendLog("没有可用域名")
			return
		}

		port, err := strconv.Atoi(strings.TrimSpace(portEd.Text()))
		if err != nil {
			appendLog("端口无效")
			return
		}
		timeoutMs, err := strconv.Atoi(strings.TrimSpace(timeoutEd.Text()))
		if err != nil {
			appendLog("超时无效")
			return
		}
		attempts, err := strconv.Atoi(strings.TrimSpace(attemptsEd.Text()))
		if err != nil {
			appendLog("次数无效")
			return
		}
		concurrency, err := strconv.Atoi(strings.TrimSpace(concurrencyEd.Text()))
		if err != nil {
			appendLog("并发无效")
			return
		}

		cfg := engine.Config{
			DNSServers:  parseTokens(dnsEd.Text()),
			Port:        port,
			Timeout:     time.Duration(timeoutMs) * time.Millisecond,
			Attempts:    attempts,
			Concurrency: concurrency,
			IPv4:        ipv4.Value,
			IPv6:        ipv6.Value,
		}

		rows = nil
		domainIdx = map[string]int{}
		logLines = nil
		logEd.SetText("")
		previewTxt = ""
		previewEd.SetText("")
		lastBackup = ""
		done, total = 0, 0

		ctx, c := context.WithCancel(context.Background())
		cancel = c
		running = true

		go func() {
			err := engine.Run(ctx, domains, cfg, engine.Callbacks{
				OnLog: func(s string) {
					select {
					case uiCh <- msgLog{Line: s}:
					default:
					}
					w.Invalidate()
				},
				OnResult: func(r model.DomainResult) {
					select {
					case uiCh <- msgResult{Result: r}:
					default:
					}
					w.Invalidate()
				},
				OnProgress: func(d, t int) {
					select {
					case uiCh <- msgProgress{Done: d, Total: t}:
					default:
					}
					w.Invalidate()
				},
			})
			select {
			case uiCh <- msgDone{Err: err}:
			default:
			}
			w.Invalidate()
		}()
	}

	stopRun := func() {
		if cancel != nil {
			cancel()
		}
	}

	loadDomainsFromHosts := func() {
		p := strings.TrimSpace(hostsEd.Text())
		if p == "" {
			p = hostsfile.DefaultHostsPath()
		}
		ds, err := domain.ReadDomainsFromHosts(p)
		if err != nil {
			appendLog("读取 hosts 失败：" + err.Error())
			return
		}
		domainsEd.SetText(strings.Join(ds, "\n"))
		appendLog(fmt.Sprintf("已导入 hosts 域名：%d", len(ds)))
	}

	pickDomainsFile := func() {
		go func() {
			p, err := filedialog.OpenFile("选择域名文件", []filedialog.Filter{
				{Name: "文本文件 (*.txt)", Pattern: "*.txt"},
				{Name: "所有文件 (*.*)", Pattern: "*.*"},
			})
			select {
			case uiCh <- msgPickedPath{Kind: "domains", Path: p, Err: err}:
			default:
			}
			w.Invalidate()
		}()
	}

	pickHostsFile := func() {
		go func() {
			p, err := filedialog.OpenFile("选择 hosts 文件", []filedialog.Filter{
				{Name: "hosts", Pattern: "hosts"},
				{Name: "所有文件 (*.*)", Pattern: "*.*"},
			})
			select {
			case uiCh <- msgPickedPath{Kind: "hosts", Path: p, Err: err}:
			default:
			}
			w.Invalidate()
		}()
	}

	buildPreview := func() {
		p := strings.TrimSpace(hostsEd.Text())
		if p == "" {
			p = hostsfile.DefaultHostsPath()
		}
		orig, err := hostsfile.Read(p)
		if err != nil {
			appendLog("读取 hosts 失败：" + err.Error())
			return
		}
		block := hostsfile.BuildManagedBlock(buildMappings())
		previewTxt = hostsfile.ApplyManagedBlock(orig, block)
		previewEd.SetText(previewTxt)
		mainTab.Value = "preview"
		appendLog("已生成预览")
		w.Invalidate()
	}

	writeHosts := func() {
		p := strings.TrimSpace(hostsEd.Text())
		if p == "" {
			p = hostsfile.DefaultHostsPath()
		}
		backup, _, err := hostsfile.WriteWithBackup(p, buildMappings())
		if err != nil {
			appendLog("写入失败：" + err.Error())
			return
		}
		lastBackup = backup
		appendLog("写入成功，备份：" + backup)
	}

	restoreHosts := func() {
		if strings.TrimSpace(lastBackup) == "" {
			appendLog("没有可恢复的备份（本次未写入）")
			return
		}
		p := strings.TrimSpace(hostsEd.Text())
		if p == "" {
			p = hostsfile.DefaultHostsPath()
		}
		if err := hostsfile.RestoreBackup(lastBackup, p); err != nil {
			appendLog("恢复失败：" + err.Error())
			return
		}
		appendLog("已恢复：" + lastBackup)
	}

	var ops op.Ops
	for {
		e := w.Event()
		switch e := e.(type) {
		case app.DestroyEvent:
			stopRun()
			return e.Err
		case app.FrameEvent:
			for {
				select {
				case m := <-uiCh:
					switch m := m.(type) {
					case msgLog:
						appendLog(m.Line)
					case msgResult:
						applyResult(m.Result)
					case msgProgress:
						done, total = m.Done, m.Total
					case msgDone:
						running = false
						if m.Err != nil && !errorsIsCanceled(m.Err) {
							appendLog("任务结束：" + m.Err.Error())
						} else {
							appendLog("任务结束")
						}
					case msgPickedPath:
						if m.Err != nil {
							if strings.Contains(strings.ToLower(m.Err.Error()), "canceled") {
								break
							}
							appendLog("选择文件失败：" + m.Err.Error())
							break
						}
						if strings.TrimSpace(m.Path) == "" {
							break
						}
						switch m.Kind {
						case "domains":
							ds, err := domain.ReadDomainsFromFile(m.Path)
							if err != nil {
								appendLog("读取文件失败：" + err.Error())
								break
							}
							domainFilePath = m.Path
							domainsEd.SetText(strings.Join(ds, "\n"))
							appendLog(fmt.Sprintf("已导入文件域名：%d (%s)", len(ds), filepath.Base(m.Path)))
						case "hosts":
							hostsEd.SetText(m.Path)
							appendLog("已选择 hosts：" + m.Path)
						}
					}
				default:
					goto drained
				}
			}
		drained:

			ops.Reset()
			gtx := app.NewContext(&ops, e)
			layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return headerBar(th, gtx, &startBtn, &stopBtn, running, done, total,
						func() {
							if !running {
								startRun()
							}
						},
						func() { stopRun() },
					)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return tabBar(th, gtx, &mainTab, &tabConfigBtn, &tabResultsBtn, &tabLogBtn, &tabPreviewBtn)
				}),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					switch mainTab.Value {
					case "results":
						return rightPanel(th, gtx, &resultsList, &selectAllBtn, &selectNoneBtn, &selectOKBtn, rows,
							func(mode string) {
								switch mode {
								case "all":
									for i := range rows {
										if rows[i].Message == "" && rows[i].BestIP != "" {
											rows[i].Apply.Value = true
										}
									}
								case "none":
									for i := range rows {
										rows[i].Apply.Value = false
									}
								case "ok":
									for i := range rows {
										rows[i].Apply.Value = rows[i].Message == "" && rows[i].BestIP != ""
									}
								}
							},
						)
					case "log":
						return editorPage(th, gtx, "日志", &logEd)
					case "preview":
						return previewPage(th, gtx, &previewEd, &previewBtn, &writeBtn, &restoreBtn,
							func() { buildPreview() },
							func() { writeHosts() },
							func() { restoreHosts() },
						)
					default:
						return leftPanel(th, gtx, &leftList, &domainsEd, &dnsEd, &hostsEd, &portEd, &timeoutEd, &attemptsEd, &concurrencyEd, &ipv4, &ipv6,
							&loadHosts, &pickFile, &pickHosts,
							running,
							domainFilePath,
							func() { loadDomainsFromHosts() },
							func() { pickDomainsFile() },
							func() { pickHostsFile() },
						)
					}
				}),
			)
			e.Frame(&ops)
		}
	}
}

func headerBar(th *material.Theme, gtx layout.Context, startBtn, stopBtn *widget.Clickable, running bool, done, total int, onStart, onStop func()) layout.Dimensions {
	gtx.Constraints.Min.Y = gtx.Dp(unit.Dp(88))
	return layout.UniformInset(uiPad).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return card(gtx, uiRadius, uiSurface, uiBorderCol, uiBorder, layout.UniformInset(uiPad), func(gtx layout.Context) layout.Dimensions {
			title := material.H6(th, "IP 优选（hosts）")
			title.Color = uiText

			var progress float32
			var progressText string
			if total > 0 {
				progress = float32(done) / float32(total)
				progressText = fmt.Sprintf("%d / %d", done, total)
			}

			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
						layout.Rigid(title.Layout),
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							if total <= 0 {
								return layout.Dimensions{}
							}
							return layout.Inset{Left: uiGap}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
									layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
										bar := material.ProgressBar(th, progress)
										return bar.Layout(gtx)
									}),
									layout.Rigid(spacer(unit.Dp(8))),
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										l := material.Caption(th, progressText)
										l.Color = uiMuted
										return l.Layout(gtx)
									}),
								)
							})
						}),
					)
				}),
				layout.Rigid(spacer(unit.Dp(6))),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions { return layout.Dimensions{} }),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return actionButton(th, gtx, startBtn, "开始", !running, uiPrimary, color.NRGBA{A: 255, R: 255, G: 255, B: 255}, onStart)
						}),
						layout.Rigid(spacer(uiGap)),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return actionButton(th, gtx, stopBtn, "停止", running, uiDanger, color.NRGBA{A: 255, R: 255, G: 255, B: 255}, onStop)
						}),
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions { return layout.Dimensions{} }),
					)
				}),
			)
		})
	})
}

func tabBar(th *material.Theme, gtx layout.Context, tab *widget.Enum, configBtn, resultsBtn, logBtn, previewBtn *widget.Clickable) layout.Dimensions {
	return layout.UniformInset(uiPad).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return tabButton(th, gtx, configBtn, tab, "config", "配置")
			}),
			layout.Rigid(spacer(unit.Dp(12))),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return tabButton(th, gtx, resultsBtn, tab, "results", "结果")
			}),
			layout.Rigid(spacer(unit.Dp(12))),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return tabButton(th, gtx, logBtn, tab, "log", "日志")
			}),
			layout.Rigid(spacer(unit.Dp(12))),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return tabButton(th, gtx, previewBtn, tab, "preview", "预览")
			}),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions { return layout.Dimensions{} }),
		)
	})
}

func tabButton(th *material.Theme, gtx layout.Context, c *widget.Clickable, tab *widget.Enum, key, label string) layout.Dimensions {
	for c.Clicked(gtx) {
		tab.Value = key
		gtx.Execute(op.InvalidateCmd{})
	}

	active := tab.Value == key
	fg := uiMuted
	if active {
		fg = uiText
	}

	gtx.Constraints.Min.Y = gtx.Dp(unit.Dp(36))
	return material.Clickable(gtx, c, func(gtx layout.Context) layout.Dimensions {
		inset := layout.Inset{Top: unit.Dp(8), Bottom: unit.Dp(8), Left: unit.Dp(6), Right: unit.Dp(6)}
		return inset.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			var labelDims layout.Dimensions
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					l := material.Body2(th, label)
					l.Color = fg
					labelDims = l.Layout(gtx)
					return labelDims
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					w := labelDims.Size.X
					if w < gtx.Dp(unit.Dp(28)) {
						w = gtx.Dp(unit.Dp(28))
					}
					h := gtx.Dp(unit.Dp(2))
					size := image.Pt(w, h)
					gtx.Constraints.Min = size
					gtx.Constraints.Max = size
					if active {
						defer clip.Rect{Max: size}.Push(gtx.Ops).Pop()
						paint.Fill(gtx.Ops, uiPrimary)
					}
					return layout.Dimensions{Size: size}
				}),
			)
		})
	})
}

func leftPanel(th *material.Theme, gtx layout.Context,
	leftList *layout.List,
	domainsEd, dnsEd, hostsEd, portEd, timeoutEd, attemptsEd, concurrencyEd *widget.Editor,
	ipv4, ipv6 *widget.Bool,
	loadHosts, pickFile, pickHosts *widget.Clickable,
	running bool,
	domainFilePath string,
	onLoadHosts, onPickFile, onPickHosts func(),
) layout.Dimensions {
	return layout.UniformInset(uiPad).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return leftList.Layout(gtx, 1, func(gtx layout.Context, _ int) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return card(gtx, uiRadius, uiSurface, uiBorderCol, uiBorder, layout.UniformInset(uiPad), func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return sectionTitle(th, gtx, "输入")
							}),
							layout.Rigid(spacer(uiGap)),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return editorBox(th, gtx, domainsEd, unit.Dp(120), "每行一个域名，支持 # 注释")
							}),
							layout.Rigid(spacer(uiGap)),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
									layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
										return actionButton(th, gtx, loadHosts, "从 hosts 读取", !running, uiSurface, uiText, onLoadHosts)
									}),
									layout.Rigid(spacer(uiGap)),
									layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
										return actionButton(th, gtx, pickFile, "选择域名文件", true, uiSurface, uiText, onPickFile)
									}),
								)
							}),
							layout.Rigid(spacer(uiGap)),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								if strings.TrimSpace(domainFilePath) == "" {
									l := material.Caption(th, "未选择域名文件（可直接在上方粘贴域名）")
									l.Color = uiMuted
									return l.Layout(gtx)
								}
								l := material.Caption(th, "已选择："+filepath.Base(domainFilePath))
								l.Color = uiMuted
								return l.Layout(gtx)
							}),
						)
					})
				}),
				layout.Rigid(spacer(uiGap)),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return card(gtx, uiRadius, uiSurface, uiBorderCol, uiBorder, layout.UniformInset(uiPad), func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return sectionTitle(th, gtx, "测速")
							}),
							layout.Rigid(spacer(uiGap)),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return editorBox(th, gtx, dnsEd, unit.Dp(78), "DNS 服务器（每行一个，可为空）")
							}),
							layout.Rigid(spacer(uiGap)),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
									layout.Flexed(1, func(gtx layout.Context) layout.Dimensions { return labeledEditor(th, gtx, "端口", portEd) }),
									layout.Rigid(spacer(uiGap)),
									layout.Flexed(1, func(gtx layout.Context) layout.Dimensions { return labeledEditor(th, gtx, "超时(ms)", timeoutEd) }),
								)
							}),
							layout.Rigid(spacer(uiGap)),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
									layout.Flexed(1, func(gtx layout.Context) layout.Dimensions { return labeledEditor(th, gtx, "次数", attemptsEd) }),
									layout.Rigid(spacer(uiGap)),
									layout.Flexed(1, func(gtx layout.Context) layout.Dimensions { return labeledEditor(th, gtx, "并发", concurrencyEd) }),
								)
							}),
							layout.Rigid(spacer(uiGap)),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
									layout.Rigid(material.CheckBox(th, ipv4, "IPv4").Layout),
									layout.Rigid(spacer(uiGap)),
									layout.Rigid(material.CheckBox(th, ipv6, "IPv6").Layout),
									layout.Flexed(1, func(gtx layout.Context) layout.Dimensions { return layout.Dimensions{} }),
								)
							}),
						)
					})
				}),
				layout.Rigid(spacer(uiGap)),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return card(gtx, uiRadius, uiSurface, uiBorderCol, uiBorder, layout.UniformInset(uiPad), func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return sectionTitle(th, gtx, "hosts")
							}),
							layout.Rigid(spacer(uiGap)),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return editorLine(th, gtx, hostsEd, "hosts 文件路径")
							}),
							layout.Rigid(spacer(uiGap)),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return actionButton(th, gtx, pickHosts, "选择 hosts 文件", true, uiSurface, uiText, onPickHosts)
							}),
							layout.Rigid(spacer(unit.Dp(6))),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								l := material.Caption(th, "预览/写入/恢复：请到「预览」页操作")
								l.Color = uiMuted
								return l.Layout(gtx)
							}),
						)
					})
				}),
			)
		})
	})
}

func previewPage(th *material.Theme, gtx layout.Context, ed *widget.Editor, previewBtn, writeBtn, restoreBtn *widget.Clickable, onPreview, onWrite, onRestore func()) layout.Dimensions {
	return layout.UniformInset(uiPad).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return card(gtx, uiRadius, uiSurface, uiBorderCol, uiBorder, layout.UniformInset(uiPad), func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return sectionTitle(th, gtx, "预览")
						}),
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions { return layout.Dimensions{} }),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return actionButton(th, gtx, previewBtn, "生成预览", true, uiSurface, uiText, onPreview)
						}),
						layout.Rigid(spacer(uiGap)),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return actionButton(th, gtx, writeBtn, "写入", true, uiPrimary, color.NRGBA{A: 255, R: 255, G: 255, B: 255}, onWrite)
						}),
						layout.Rigid(spacer(uiGap)),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return actionButton(th, gtx, restoreBtn, "恢复备份", true, uiSurface, uiText, onRestore)
						}),
					)
				}),
				layout.Rigid(spacer(uiGap)),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints.Min.Y = gtx.Constraints.Max.Y
					e := material.Editor(th, ed, "")
					e.TextSize = unit.Sp(14)
					e.Color = uiText
					e.HintColor = uiMuted
					e.LineHeightScale = 1.25
					return card(gtx, uiRadiusSmall, uiSurface, uiBorderCol, uiBorder, layout.UniformInset(unit.Dp(10)), e.Layout)
				}),
			)
		})
	})
}

func rightPanel(th *material.Theme, gtx layout.Context, list *layout.List, selectAllBtn, selectNoneBtn, selectOKBtn *widget.Clickable, rows []row, onSelect func(mode string)) layout.Dimensions {
	return layout.UniformInset(uiPad).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return card(gtx, uiRadius, uiSurface, uiBorderCol, uiBorder, layout.UniformInset(uiPad), func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							lbl := material.H6(th, "结果")
							lbl.Color = uiText
							return lbl.Layout(gtx)
						}),
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions { return layout.Dimensions{} }),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return actionButton(th, gtx, selectAllBtn, "全选", true, uiSurface, uiText, func() { onSelect("all") })
						}),
						layout.Rigid(spacer(uiGap)),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return actionButton(th, gtx, selectNoneBtn, "全不选", true, uiSurface, uiText, func() { onSelect("none") })
						}),
						layout.Rigid(spacer(uiGap)),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return actionButton(th, gtx, selectOKBtn, "只选成功", true, uiSurface, uiText, func() { onSelect("ok") })
						}),
					)
				})
			}),
			layout.Rigid(spacer(uiGap)),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return card(gtx, uiRadius, uiSurface, uiBorderCol, uiBorder, layout.UniformInset(uiPad), func(gtx layout.Context) layout.Dimensions {
					return list.Layout(gtx, len(rows), func(gtx layout.Context, i int) layout.Dimensions {
						r := rows[i]
						return resultRow(th, gtx, &rows[i], r)
					})
				})
			}),
		)
	})
}

func editorPage(th *material.Theme, gtx layout.Context, title string, ed *widget.Editor) layout.Dimensions {
	return layout.UniformInset(uiPad).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return card(gtx, uiRadius, uiSurface, uiBorderCol, uiBorder, layout.UniformInset(uiPad), func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return sectionTitle(th, gtx, title)
				}),
				layout.Rigid(spacer(uiGap)),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints.Min.Y = gtx.Constraints.Max.Y
					e := material.Editor(th, ed, "")
					e.TextSize = unit.Sp(14)
					e.Color = uiText
					e.HintColor = uiMuted
					e.LineHeightScale = 1.25
					return card(gtx, uiRadiusSmall, uiSurface, uiBorderCol, uiBorder, layout.UniformInset(unit.Dp(10)), e.Layout)
				}),
			)
		})
	})
}

func resultRow(th *material.Theme, gtx layout.Context, target *row, r row) layout.Dimensions {
	return layout.Inset{Bottom: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		bg := uiSurface
		if strings.TrimSpace(r.Message) != "" {
			bg = color.NRGBA{A: 255, R: 255, G: 248, B: 248}
		}
		return card(gtx, uiRadiusSmall, bg, uiBorderCol, uiBorder, layout.UniformInset(unit.Dp(10)), func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
						layout.Rigid(material.CheckBox(th, &target.Apply, "").Layout),
						layout.Rigid(spacer(unit.Dp(8))),
						layout.Flexed(0.55, func(gtx layout.Context) layout.Dimensions {
							l := material.Body1(th, r.Domain)
							l.Color = uiText
							return l.Layout(gtx)
						}),
						layout.Flexed(0.25, func(gtx layout.Context) layout.Dimensions {
							l := material.Body1(th, r.BestIP)
							l.Color = uiText
							return l.Layout(gtx)
						}),
						layout.Flexed(0.20, func(gtx layout.Context) layout.Dimensions {
							var s string
							if r.BestIP != "" {
								s = fmt.Sprintf("%.0f%%  %s", r.Rate*100, r.P95)
							}
							l := material.Caption(th, s)
							l.Color = uiMuted
							return l.Layout(gtx)
						}),
					)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if strings.TrimSpace(r.Message) == "" {
						return layout.Dimensions{}
					}
					l := material.Caption(th, r.Message)
					l.Color = uiDanger
					l.Alignment = text.Start
					return layout.Inset{Top: unit.Dp(6)}.Layout(gtx, l.Layout)
				}),
			)
		})
	})
}

func editorBox(th *material.Theme, gtx layout.Context, ed *widget.Editor, height unit.Dp, hint string) layout.Dimensions {
	gtx.Constraints.Min.Y = gtx.Dp(height)
	gtx.Constraints.Max.Y = gtx.Dp(height)
	e := material.Editor(th, ed, hint)
	e.TextSize = unit.Sp(14)
	e.Color = uiText
	e.HintColor = uiMuted
	e.LineHeightScale = 1.25
	return card(gtx, uiRadiusSmall, uiSurface, uiBorderCol, uiBorder, layout.UniformInset(unit.Dp(10)), e.Layout)
}

func editorLine(th *material.Theme, gtx layout.Context, ed *widget.Editor, hint string) layout.Dimensions {
	gtx.Constraints.Min.Y = gtx.Dp(uiCtrlH)
	e := material.Editor(th, ed, hint)
	e.TextSize = unit.Sp(14)
	e.Color = uiText
	e.HintColor = uiMuted
	e.LineHeightScale = 1.1
	return card(gtx, uiRadiusSmall, uiSurface, uiBorderCol, uiBorder, layout.UniformInset(unit.Dp(10)), e.Layout)
}

func labeledEditor(th *material.Theme, gtx layout.Context, label string, ed *widget.Editor) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			l := material.Caption(th, label)
			l.Color = uiMuted
			return l.Layout(gtx)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return editorLine(th, gtx, ed, "") }),
	)
}

func spacer(h unit.Dp) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		return layout.Spacer{Width: h, Height: h}.Layout(gtx)
	}
}

func errorsIsCanceled(err error) bool {
	return errors.Is(err, context.Canceled) || strings.Contains(strings.ToLower(err.Error()), "canceled")
}

func parseTokens(text string) []string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	text = strings.ReplaceAll(text, ",", " ")
	text = strings.ReplaceAll(text, ";", " ")
	return strings.Fields(strings.ReplaceAll(text, "\n", " "))
}

func actionButton(th *material.Theme, gtx layout.Context, c *widget.Clickable, label string, enabled bool, bg, fg color.NRGBA, onClick ...func()) layout.Dimensions {
	gtx.Constraints.Min.Y = gtx.Dp(uiCtrlH)
	btn := material.Button(th, c, label)
	btn.CornerRadius = uiRadiusSmall
	btn.TextSize = unit.Sp(14)
	btn.Background = bg
	btn.Color = fg
	btn.Inset = layout.Inset{Top: unit.Dp(8), Bottom: unit.Dp(8), Left: unit.Dp(14), Right: unit.Dp(14)}
	if !enabled {
		btn.Background = color.NRGBA{A: 255, R: 238, G: 239, B: 242}
		btn.Color = color.NRGBA{A: 255, R: 150, G: 154, B: 162}
		gtx = gtx.Disabled()
	}
	for enabled && c.Clicked(gtx) {
		if len(onClick) > 0 && onClick[0] != nil {
			onClick[0]()
		}
	}
	return btn.Layout(gtx)
}

func sectionTitle(th *material.Theme, gtx layout.Context, title string) layout.Dimensions {
	l := material.Subtitle1(th, title)
	l.Color = uiText
	return l.Layout(gtx)
}

func card(gtx layout.Context, radius unit.Dp, bg, border color.NRGBA, borderWidth unit.Dp, inset layout.Inset, w layout.Widget) layout.Dimensions {
	m := op.Record(gtx.Ops)
	dims := inset.Layout(gtx, w)
	call := m.Stop()

	size := dims.Size
	r := gtx.Dp(radius)
	bw := gtx.Dp(borderWidth)
	r2 := int(math.Max(0, float64(r-bw)))

	outer := clip.RRect{Rect: image.Rectangle{Max: size}, NE: r, NW: r, SE: r, SW: r}.Op(gtx.Ops)
	paint.FillShape(gtx.Ops, border, outer)

	innerRect := image.Rectangle{Max: image.Pt(max0(size.X-2*bw), max0(size.Y-2*bw))}
	inner := clip.RRect{Rect: innerRect, NE: r2, NW: r2, SE: r2, SW: r2}.Op(gtx.Ops)
	st := op.Offset(image.Pt(bw, bw)).Push(gtx.Ops)
	paint.FillShape(gtx.Ops, bg, inner)
	st.Pop()

	call.Add(gtx.Ops)
	return dims
}

func max0(v int) int {
	if v < 0 {
		return 0
	}
	return v
}
