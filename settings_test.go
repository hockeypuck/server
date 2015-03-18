package server

import (
	"encoding/json"
	"testing"

	gc "gopkg.in/check.v1"
	"gopkg.in/hockeypuck/conflux.v2/recon"
)

func Test(t *testing.T) { gc.TestingT(t) }

type SettingsSuite struct{}

var _ = gc.Suite(&SettingsSuite{})

const fnord = "fnord"

func (s *SettingsSuite) TestFnords(c *gc.C) {
	settings := DefaultSettings()
	settings.Conflux.Recon.Settings.LogName = fnord
	settings.Conflux.Recon.Settings.HTTPNet = fnord
	settings.Conflux.Recon.Settings.ReconNet = fnord
	settings.Conflux.Recon.Settings.CompatHTTPPort = 23
	settings.Conflux.Recon.Settings.CompatReconPort = 23
	settings.Conflux.Recon.Settings.CompatPartnerAddrs = []string{fnord}
	settings.Conflux.Recon.GossipIntervalSecs = 23
	settings.Conflux.Recon.MaxOutstandingReconRequests = 23
	settings.Conflux.Recon.Partners["weishaupt"] = recon.Partner{HTTPNet: fnord, ReconNet: "fnord"}
	settings.Conflux.Recon.LevelDB.Path = fnord
	settings.IndexTemplate = fnord
	settings.VIndexTemplate = fnord
	settings.StatsTemplate = fnord
	settings.HKPS = &HKPSConfig{Key: fnord}
	settings.OpenPGP.PKS = &PKSConfig{SMTP: SMTPConfig{fnord, fnord, fnord, fnord}}
	settings.OpenPGP.DB.DSN = fnord
	settings.OpenPGP.DB.Mongo = &mongoConfig{fnord, fnord}
	settings.LogFile = fnord
	settings.LogLevel = fnord
	settings.Webroot = fnord

	out, err := json.Marshal(&settings)
	c.Assert(err, gc.IsNil)
	c.Assert(string(out), gc.Matches, ".*weishaupt.*")
	c.Assert(string(out), gc.Not(gc.Matches), ".*fnord.*")
	c.Assert(string(out), gc.Not(gc.Matches), ".*23.*")
}
