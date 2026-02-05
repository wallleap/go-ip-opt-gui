package engine

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net"
	"net/netip"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"example.com/ip-opt-gui/internal/model"
)

type Config struct {
	DNSServers  []string
	Port        int
	Timeout     time.Duration
	Attempts    int
	Concurrency int
	IPv4        bool
	IPv6        bool
}

func (c Config) validate() error {
	if c.Port <= 0 || c.Port > 65535 {
		return errors.New("invalid port")
	}
	if c.Timeout <= 0 {
		return errors.New("invalid timeout")
	}
	if c.Attempts <= 0 {
		return errors.New("invalid attempts")
	}
	if c.Concurrency <= 0 {
		return errors.New("invalid concurrency")
	}
	if !c.IPv4 && !c.IPv6 {
		return errors.New("select ipv4 and/or ipv6")
	}
	return nil
}

type Callbacks struct {
	OnLog      func(string)
	OnResult   func(model.DomainResult)
	OnProgress func(done, total int)
}

func Run(ctx context.Context, domains []string, cfg Config, cb Callbacks) error {
	if err := cfg.validate(); err != nil {
		return err
	}
	if len(domains) == 0 {
		return errors.New("empty domain list")
	}

	total := len(domains)
	var done int64
	if cb.OnProgress != nil {
		cb.OnProgress(0, total)
	}

	workCh := make(chan string)
	var wg sync.WaitGroup

	worker := func() {
		defer wg.Done()
		for domain := range workCh {
			res := RunOneDomain(ctx, domain, cfg, cb.OnLog)
			if cb.OnResult != nil {
				cb.OnResult(res)
			}
			d := int(atomic.AddInt64(&done, 1))
			if cb.OnProgress != nil {
				cb.OnProgress(d, total)
			}
		}
	}

	for i := 0; i < cfg.Concurrency; i++ {
		wg.Add(1)
		go worker()
	}

	for _, d := range domains {
		select {
		case <-ctx.Done():
			close(workCh)
			wg.Wait()
			return ctx.Err()
		case workCh <- d:
		}
	}
	close(workCh)
	wg.Wait()
	return nil
}

func RunOneDomain(ctx context.Context, domain string, cfg Config, logf func(string)) model.DomainResult {
	res := model.DomainResult{Domain: domain}

	candidates, err := ResolveCandidates(ctx, domain, cfg.DNSServers, cfg.IPv4, cfg.IPv6)
	if err != nil {
		res.Err = err
		return res
	}
	if len(candidates) == 0 {
		res.Err = errors.New("no candidate ip")
		return res
	}

	stats := make([]model.CandidateStat, 0, len(candidates))
	for _, c := range candidates {
		if ctx.Err() != nil {
			res.Err = ctx.Err()
			return res
		}
		st := ProbeCandidate(ctx, c.IP, cfg.Port, cfg.Timeout, cfg.Attempts)
		st.ResolvedVia = c.ResolvedVia
		stats = append(stats, st)
		if logf != nil {
			logf(fmt.Sprintf("%s -> %s (success %.0f%%, p95 %s)", domain, st.IP.String(), st.SuccessRate()*100, st.P95))
		}
	}

	sort.Slice(stats, func(i, j int) bool { return better(stats[i], stats[j]) })
	res.Candidates = stats
	res.Best = stats[0]
	return res
}

type Candidate struct {
	IP          netip.Addr
	ResolvedVia string
}

func ResolveCandidates(ctx context.Context, domain string, servers []string, ipv4, ipv6 bool) ([]Candidate, error) {
	type resolved struct {
		ip  netip.Addr
		via string
	}
	seen := map[netip.Addr]string{}

	addIPs := func(via string, ips []netip.Addr) {
		for _, ip := range ips {
			if ip.IsValid() && !ip.IsUnspecified() {
				if _, ok := seen[ip]; !ok {
					seen[ip] = via
				}
			}
		}
	}

	sysIPs, _ := lookupWithResolver(ctx, net.DefaultResolver, domain)
	addIPs("system", filterIPVersions(sysIPs, ipv4, ipv6))

	for _, s := range servers {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		r := resolverForServer(s)
		ips, err := lookupWithResolver(ctx, r, domain)
		if err != nil {
			continue
		}
		addIPs(s, filterIPVersions(ips, ipv4, ipv6))
	}

	var out []Candidate
	for ip, via := range seen {
		out = append(out, Candidate{IP: ip, ResolvedVia: via})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].IP.Less(out[j].IP) })
	return out, nil
}

func ProbeCandidate(ctx context.Context, ip netip.Addr, port int, timeout time.Duration, attempts int) model.CandidateStat {
	st := model.CandidateStat{IP: ip}
	for i := 0; i < attempts; i++ {
		if ctx.Err() != nil {
			st.LastError = ctx.Err().Error()
			break
		}
		d, err := tcpPing(ctx, ip, port, timeout)
		if err != nil {
			st.Failures++
			st.LastError = err.Error()
			continue
		}
		st.Successes++
		st.Samples = append(st.Samples, d)
	}

	if len(st.Samples) > 0 {
		st.P50 = quantile(st.Samples, 0.50)
		st.P95 = quantile(st.Samples, 0.95)
		st.JitterStd = stddev(st.Samples)
	} else {
		st.P50 = timeout
		st.P95 = timeout
		st.JitterStd = timeout
	}
	return st
}

func better(a, b model.CandidateStat) bool {
	ar, br := a.SuccessRate(), b.SuccessRate()
	if ar != br {
		return ar > br
	}
	if a.P95 != b.P95 {
		return a.P95 < b.P95
	}
	if a.P50 != b.P50 {
		return a.P50 < b.P50
	}
	if a.JitterStd != b.JitterStd {
		return a.JitterStd < b.JitterStd
	}
	return a.IP.Less(b.IP)
}

func tcpPing(ctx context.Context, ip netip.Addr, port int, timeout time.Duration) (time.Duration, error) {
	address := net.JoinHostPort(ip.String(), fmt.Sprintf("%d", port))
	dialer := net.Dialer{Timeout: timeout}
	start := time.Now()
	conn, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return 0, err
	}
	_ = conn.Close()
	return time.Since(start), nil
}

func resolverForServer(server string) *net.Resolver {
	addr := normalizeDNSServer(server)
	dialer := net.Dialer{Timeout: 3 * time.Second}
	return &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			return dialer.DialContext(ctx, "udp", addr)
		},
	}
}

func normalizeDNSServer(server string) string {
	server = strings.TrimSpace(server)
	if server == "" {
		return ""
	}
	if _, _, err := net.SplitHostPort(server); err == nil {
		return server
	}
	if ip, err := netip.ParseAddr(server); err == nil {
		return net.JoinHostPort(ip.String(), "53")
	}
	return net.JoinHostPort(server, "53")
}

func lookupWithResolver(ctx context.Context, r *net.Resolver, domain string) ([]netip.Addr, error) {
	addrs, err := r.LookupIPAddr(ctx, domain)
	if err != nil {
		return nil, err
	}
	out := make([]netip.Addr, 0, len(addrs))
	for _, a := range addrs {
		if a.IP == nil {
			continue
		}
		if ip, ok := netip.AddrFromSlice(a.IP); ok {
			out = append(out, ip)
		}
	}
	return out, nil
}

func filterIPVersions(ips []netip.Addr, ipv4, ipv6 bool) []netip.Addr {
	out := ips[:0]
	for _, ip := range ips {
		if ip.Is4() && ipv4 {
			out = append(out, ip)
		}
		if ip.Is6() && ipv6 {
			out = append(out, ip)
		}
	}
	return out
}

func quantile(samples []time.Duration, q float64) time.Duration {
	if len(samples) == 0 {
		return 0
	}
	cp := append([]time.Duration(nil), samples...)
	sort.Slice(cp, func(i, j int) bool { return cp[i] < cp[j] })
	if q <= 0 {
		return cp[0]
	}
	if q >= 1 {
		return cp[len(cp)-1]
	}
	pos := q * float64(len(cp)-1)
	idx := int(math.Floor(pos))
	frac := pos - float64(idx)
	if idx >= len(cp)-1 {
		return cp[len(cp)-1]
	}
	a, b := cp[idx], cp[idx+1]
	return time.Duration(float64(a) + (float64(b)-float64(a))*frac)
}

func stddev(samples []time.Duration) time.Duration {
	if len(samples) == 0 {
		return 0
	}
	var sum float64
	for _, s := range samples {
		sum += float64(s)
	}
	mean := sum / float64(len(samples))
	var v float64
	for _, s := range samples {
		d := float64(s) - mean
		v += d * d
	}
	v /= float64(len(samples))
	return time.Duration(math.Sqrt(v))
}

