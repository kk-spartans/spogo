package spotify

import "time"

const commandRouteTTL = 10 * time.Minute

func (c *ConnectClient) cacheCommandRoute(state connectState) {
	if c == nil || state.activeDeviceID == "" {
		return
	}
	c.routeMu.Lock()
	c.cachedActiveDeviceID = state.activeDeviceID
	c.cachedOriginDeviceID = state.originDeviceID
	c.cachedRouteAt = time.Now()
	c.routeMu.Unlock()
}

func (c *ConnectClient) commandRoute() (string, string, bool) {
	if c == nil {
		return "", "", false
	}
	c.routeMu.RLock()
	active := c.cachedActiveDeviceID
	origin := c.cachedOriginDeviceID
	at := c.cachedRouteAt
	c.routeMu.RUnlock()
	if active == "" || at.IsZero() || time.Since(at) > commandRouteTTL {
		return "", "", false
	}
	from := origin
	if from == "" {
		c.session.mu.Lock()
		from = c.session.connectDeviceID
		c.session.mu.Unlock()
	}
	if from == "" {
		from = active
	}
	if from == "" {
		return "", "", false
	}
	return from, active, true
}

func (c *ConnectClient) invalidateCommandRoute() {
	if c == nil {
		return
	}
	c.routeMu.Lock()
	c.cachedActiveDeviceID = ""
	c.cachedOriginDeviceID = ""
	c.cachedRouteAt = time.Time{}
	c.routeMu.Unlock()
}
