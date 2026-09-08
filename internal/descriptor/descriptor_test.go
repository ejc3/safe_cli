package descriptor

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"
)

// emptyObjRe matches an empty JSON object as a value, e.g. `"locationDetails": {}` — the
// classic "inferred but never filled in" hole.
var emptyObjRe = regexp.MustCompile(`:\s*\{\s*\}`)

// TestEveryBodyOpHasACompleteExample is the completeness gate: every op that declares a JSON
// request body MUST carry a body_example that is real — non-empty, valid JSON, with no empty
// {} object (bare or nested) and not the bare {"id":...} stub. A single-field body like
// {"mdn":"5551234567"} is fine; length is not the test. This fails hard if any body-taking op
// regresses to a stub/empty/missing body, so "comprehensive" is enforced by CI, not by claim.
func TestEveryBodyOpHasACompleteExample(t *testing.T) {
	d, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	check := func(name string, op Operation) {
		if !op.TakesBody || op.Multipart {
			return
		}
		ex := strings.TrimSpace(op.BodyExample)
		if ex == "" {
			t.Errorf("%s takes a JSON body but has no body_example", name)
			return
		}
		var v any
		if err := json.Unmarshal([]byte(ex), &v); err != nil {
			t.Errorf("%s body_example is not valid JSON: %v", name, err)
			return
		}
		// A real request body is a non-empty JSON object; null / [] / a scalar all parse as
		// valid JSON but are not a DTO, so reject them explicitly (otherwise the checks below
		// are skipped and the gate silently passes an empty/non-DTO body).
		m, ok := v.(map[string]any)
		if !ok {
			t.Errorf("%s body_example must be a JSON object (a DTO), got %T: %s", name, v, ex)
			return
		}
		if len(m) == 0 || emptyObjRe.MatchString(ex) {
			t.Errorf("%s body_example has an empty {} object (unfilled): %s", name, ex)
		}
		if len(m) == 1 {
			if _, idOnly := m["id"]; idOnly {
				t.Errorf("%s body_example is the bare {\"id\":...} stub, not the real DTO: %s", name, ex)
			}
		}
	}
	for _, en := range d.EntityNames() {
		e, _ := d.Entity(en)
		for _, n := range e.OperationNames() {
			check(en+"."+n, e.Operations[n])
		}
		for _, n := range e.ActionNames() {
			check(en+"."+n+" (action)", e.Actions[n])
		}
	}
}

// dynQueryLiteral matches anything that is not a bare wire query key: a Java reflection type
// string ("<QueryMap: HashMap<String,String>>", "(@QueryMap dynamic)"), or a "key=value"
// string / whitespace jammed into the NAME list (the harvester wrote the @QueryMap parameter
// TYPE, or baked constants, instead of the real key). A real query key is a bare identifier
// (letters/digits/-/_), so '=', '<', '>', and whitespace never belong in one; fixed values
// ride in the path, not the query name-list. Real keys were mined from the decompiled callers.
var dynQueryLiteral = regexp.MustCompile(`[<>=\s]|QueryMap|HashMap|Map<|dynamic`)

// TestNoDynamicQueryLiterals fails if any op's declared query params carry anything but a bare
// wire key. RED against the pre-fix descriptor (12 ops carried "<QueryMap: …>" placeholders;
// several others jammed "eventType=pickMeUp"/"operation=configGeoDevice" into the name-list).
func TestNoDynamicQueryLiterals(t *testing.T) {
	d, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	check := func(name string, op Operation) {
		for _, q := range op.Query {
			if dynQueryLiteral.MatchString(q) {
				t.Errorf("%s query entry is not a bare wire key (no '='/'<'/'>'/space; fixed values ride in the path): %q", name, q)
			}
		}
	}
	for _, en := range d.EntityNames() {
		e, _ := d.Entity(en)
		for _, n := range e.OperationNames() {
			check(en+"."+n, e.Operations[n])
		}
		for _, n := range e.ActionNames() {
			check(en+"."+n+" (action)", e.Actions[n])
		}
	}
}

// TestBakedFixedQueryValues locks in the decompile-mined fixed query values (workflow
// wf_f3f1d709) that were baked into op paths so agents don't have to guess them, and asserts
// the corresponding keys left the `query` name-list. RED against the pre-fix descriptor (the
// paths carried none of these and the keys were still bare names). Live-verified: each op was
// 400 without these values and 200 with them.
func TestBakedFixedQueryValues(t *testing.T) {
	d, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	op := func(en, name string) Operation {
		e, _ := d.Entity(en)
		if o, ok := e.Operations[name]; ok {
			return o
		}
		return e.Actions[name]
	}
	wantInPath := []struct{ en, name, frag string }{
		{"content_filter", "getCategories", "categorySupported=v7"},
		{"content_filter", "getCategories", "include-description=false"},
		{"content_filter", "getFilterContent", "include-description=true"},
		{"calls_and_texts", "getCallAndTextActivityListV7", "activityType=call,text"},
		{"calls_and_texts", "getCallAndTextActivityListV7", "timezone=preferred"},
		{"calls_and_texts", "getCallAndTextProfileSummaryListV7", "summaryOnly=true"},
		{"contacts", "getGizmoContacts", "contactType=gizmoContact"},
		{"contacts", "getGizmoContacts", "filterPermissions=True"},
		{"contacts", "getAllContactsRequest", "contactType=all"},
		{"restricted_usage", "getAllTheLimits", "limitType=all"},
		// location / permissions / keys ops (PR #36) — each fixed value live-verified.
		{"location", "getLocationSharingSettings", "eventType=locationSharing"},
		{"location", "getWithWhomIamSharingLocation", "eventType=onlySharing"},
		{"location", "getLocationSharingConfigEvent", "eventType=allProfiles"},
		{"location", "getPickMeUpStatus", "eventType=pickMeUp"},
		{"location", "getPickMeUpStatus", "eventStatus=active"},
		{"location", "getAvailableParentForPickMeUp", "eventType=pickMeUp"},
		{"location", "getAvailableParentForPickMeUp", "operation=getAvailableParents"},
		{"location", "sendGeoFenceConfirmation", "operation=configGeoDevice"},
		{"geofence", "updateDeviceGeofenceSettings", "operation=configGeoDevice"},
		{"feature_permissions", "getParentalControlFeaturePermissions", "featureGroup=parentalcontrols"},
		{"pairing", "getWebAppVisibility", "vpn-retry-type=BACKGROUND_REFRESH"},
		{"vpn_status", "getWebAppVisibility", "vpn-retry-type=BACKGROUND_REFRESH"},
		{"age_verification", "getSdkLicenseKey", "keys=MITEK_ANDROID_LICENSEKEY"},
		{"config", "getServiceKeys", "keys=HERE_MAP_ANDROID_PARENT_ACCESS_KEY_ID"},
		{"config", "getServiceKeys", "MB_ANDROID_CHILD"},
	}
	for _, w := range wantInPath {
		if p := op(w.en, w.name).Path; !strings.Contains(p, w.frag) {
			t.Errorf("%s.%s path must bake %q (was 400 without it); path=%q", w.en, w.name, w.frag, p)
		}
	}
	// The two gizmo-contacts ops share a route but declare DIFFERENT params
	// (ContactManagementApi.java:215/223): getFamilyMembersAndBuddies takes contactType +
	// filterAvailableMdns; getGizmoContacts takes contactType + filterPermissions. Neither may
	// carry the other's param (Codex #35 — an empty filterPermissions= leaked onto the former).
	fam := op("contacts", "getFamilyMembersAndBuddies").Path
	if !strings.Contains(fam, "filterAvailableMdns=True") || strings.Contains(fam, "filterPermissions") {
		t.Errorf("getFamilyMembersAndBuddies must bake filterAvailableMdns=True and NOT filterPermissions; path=%q", fam)
	}
	giz := op("contacts", "getGizmoContacts").Path
	if !strings.Contains(giz, "filterPermissions=True") || strings.Contains(giz, "filterAvailableMdns") {
		t.Errorf("getGizmoContacts must bake filterPermissions=True and NOT filterAvailableMdns; path=%q", giz)
	}

	// The baked keys must NOT also remain in the query name-list (that would double-send them).
	baked := map[string][]string{
		"content_filter.getCategories":                     {"categorySupported", "include-description", "r", "group-name"},
		"calls_and_texts.getAllTrustBlockWatchContactList": {"contactType"},
		"restricted_usage.getAllTheLimits":                 {"limitType"},
	}
	for key, keys := range baked {
		parts := strings.SplitN(key, ".", 2)
		for _, q := range op(parts[0], parts[1]).Query {
			for _, k := range keys {
				if q == k {
					t.Errorf("%s still lists baked key %q in query (should ride in the path only)", key, k)
				}
			}
		}
	}
}

// TestLiveCapturedBodies locks in the mutation bodies corrected from live eCapture ground
// truth (emulator, 2026-08-28) where the decompile gave the right shape but wrong values:
// scheduled-alert eventType is "locationAlert" (not "schedule_alert"), productType is lowercase
// "vsf", and updateLocationSharingSetting carries locationSharingConfig with no eventType. Each
// was round-tripped through the CLI against prod (post 201 / delete 200 / sharing PUT 200). RED
// against the pre-capture descriptor.
func TestLiveCapturedBodies(t *testing.T) {
	d, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	op := func(en, name string) Operation {
		e, _ := d.Entity(en)
		if o, ok := e.Operations[name]; ok {
			return o
		}
		return e.Actions[name]
	}
	post := op("schedule_alert", "postScheduleAlert")
	if !post.Confirmed {
		t.Error("schedule_alert.postScheduleAlert must be confirmed (verified live)")
	}
	for _, want := range []string{`"eventType":"locationAlert"`, `"productType":"vsf"`, `"alertTime"`, `"everyDay"`} {
		if !strings.Contains(post.BodyExample, want) {
			t.Errorf("postScheduleAlert body missing %q: %s", want, post.BodyExample)
		}
	}
	if strings.Contains(post.BodyExample, "schedule_alert") || strings.Contains(post.BodyExample, `"VSF"`) {
		t.Errorf("postScheduleAlert still carries the wrong pre-capture values: %s", post.BodyExample)
	}
	del := op("schedule_alert", "deleteScheduledAlert")
	if !del.Confirmed || !strings.Contains(del.BodyExample, `"eventType":"locationAlert"`) || strings.Contains(del.BodyExample, "eventDetails") {
		t.Errorf("deleteScheduledAlert must be confirmed with an eventId body and no eventDetails: confirmed=%v ex=%s", del.Confirmed, del.BodyExample)
	}
	share := op("location", "updateLocationSharingSetting")
	if !share.Confirmed || !strings.Contains(share.BodyExample, "locationSharingConfig") || strings.Contains(share.BodyExample, "eventType") {
		t.Errorf("updateLocationSharingSetting must be confirmed with locationSharingConfig and no eventType: confirmed=%v ex=%s", share.Confirmed, share.BodyExample)
	}
}

// TestReportSettingsConfirmed locks in the report/alert-settings mutations verified live
// (2026-08-28) against /vsf/commsplatform/v5/report-settings: postObjectionableSettings was
// flipped false->200->read False->restored, and postReportSetting/postReportSettings were
// no-op re-posts of the read state (200). All share the endpoint + {"settings":{...}} body.
func TestReportSettingsConfirmed(t *testing.T) {
	d, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range [][2]string{{"content_filter", "postObjectionableSettings"}, {"dashboard", "postReportSetting"}, {"dashboard", "postReportSettings"}} {
		e, _ := d.Entity(w[0])
		o := e.Operations[w[1]]
		if !o.Confirmed {
			t.Errorf("%s.%s must be confirmed (verified live)", w[0], w[1])
		}
		if !strings.Contains(o.BodyExample, `"settings"`) {
			t.Errorf("%s.%s body must wrap settings: %s", w[0], w[1], o.BodyExample)
		}
	}
}

// TestLiveCapturedBodies2 locks in the schedules/usage-limit mutations captured live via
// eCapture (2026-08-28) and round-tripped through the CLI (postSchedule 201 -> deleteSchedule
// 200; addLimit 200 -> resetLimit 200): postSchedule create is a minimal schedules[] with no
// read-only fields, and the usage limitType is data|text|call (the pre-capture descriptor had
// SCREEN_TIME). RED against the pre-capture descriptor.
func TestLiveCapturedBodies2(t *testing.T) {
	d, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	op := func(en, name string) Operation {
		e, _ := d.Entity(en)
		if o, ok := e.Operations[name]; ok {
			return o
		}
		return e.Actions[name]
	}
	ps := op("schedules", "postSchedule")
	if !ps.Confirmed || !strings.Contains(ps.BodyExample, `"scheduleType":"cus"`) || !strings.Contains(ps.BodyExample, `"notifyMember"`) || strings.Contains(ps.BodyExample, "createdAt") {
		t.Errorf("postSchedule must be confirmed, a minimal create body (cus + notifyMember, no createdAt): confirmed=%v ex=%s", ps.Confirmed, ps.BodyExample)
	}
	al := op("restricted_usage", "addLimit")
	if !al.Confirmed || !strings.Contains(al.BodyExample, `"limitType":"data"`) || strings.Contains(al.BodyExample, "SCREEN_TIME") {
		t.Errorf("addLimit must be confirmed with limitType data (not SCREEN_TIME): confirmed=%v ex=%s", al.Confirmed, al.BodyExample)
	}
	for _, w := range [][2]string{{"restricted_usage", "resetLimit"}, {"schedules", "deleteSchedule"}} {
		if !op(w[0], w[1]).Confirmed {
			t.Errorf("%s.%s must be confirmed (verified live)", w[0], w[1])
		}
	}
	// the schedules.deleteScheduledAlert alias shares the canonical (confirmed) op's route+body,
	// so it must carry the real body AND be confirmed (Codex #40 — describe --json must not
	// misclassify a verified op just because it is the aliased copy).
	alias := op("schedules", "deleteScheduledAlert")
	if strings.Contains(alias.BodyExample, `"CREATE"`) || strings.Contains(alias.BodyExample, `"VSF"`) {
		t.Errorf("schedules.deleteScheduledAlert still has the bogus pre-capture body: %s", alias.BodyExample)
	}
	if !alias.Confirmed {
		t.Error("schedules.deleteScheduledAlert alias must be confirmed (same verified route+body as the canonical)")
	}
}

// TestScreenTimeCaptured locks in the screen-time mutations verified live via CLI round-trip
// (2026-08-28): POST create (200) has NO screenTimeLimitId; PUT update (200) carries it; DELETE
// (200) is by screenTimeLimitId query. timeZone is a short zone code (the pre-capture bodies had
// screenTimeLimitId on the create and an IANA timezone). RED against the pre-capture descriptor.
func TestScreenTimeCaptured(t *testing.T) {
	d, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	e, _ := d.Entity("schedules")
	post := e.Operations["postScreenTimeData"]
	if !post.Confirmed || strings.Contains(post.BodyExample, "screenTimeLimitId") || !strings.Contains(post.BodyExample, `"timeZone":"EST"`) {
		t.Errorf("postScreenTimeData must be confirmed, no screenTimeLimitId, short-code timeZone: confirmed=%v ex=%s", post.Confirmed, post.BodyExample)
	}
	put := e.Operations["putScreenTimeData"]
	if !put.Confirmed || !strings.Contains(put.BodyExample, "screenTimeLimitId") || strings.Contains(put.BodyExample, "America/") {
		t.Errorf("putScreenTimeData must be confirmed, carry screenTimeLimitId, no IANA tz: confirmed=%v ex=%s", put.Confirmed, put.BodyExample)
	}
	if !e.Operations["deleteScreenTimeData"].Confirmed {
		t.Error("deleteScreenTimeData must be confirmed (verified live)")
	}
}

// TestCreateGroupPolicyCaptured locks in the age-group preset op verified live via eCapture +
// CLI (2026-08-28, groupId:1 -> 200): the body is a minimal {"groupId":N} (1=No filters,
// 2=Young child, 3=Child, 4=Teen). The pre-capture body had a bogus groupId:123. RED against it.
func TestCreateGroupPolicyCaptured(t *testing.T) {
	d, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	e, _ := d.Entity("content_filter")
	o := e.Operations["createGroupPolicy"]
	if !o.Confirmed || strings.Contains(o.BodyExample, "123") || !strings.Contains(o.BodyExample, `"groupId"`) {
		t.Errorf("createGroupPolicy must be confirmed with a real {\"groupId\":N} body (no 123): confirmed=%v ex=%s", o.Confirmed, o.BodyExample)
	}
}

// TestContactsCaptured locks in the contacts mutations captured live via eCapture (2026-08-28):
// putPrivateRestrictedCall contactType is "blockPrivateRestricted" (not "PRIVATE"), and
// addContactToTheList contactType is a lowercase enum (blocked|trusted|watchlist), not "TRUSTED".
// RED against the pre-capture descriptor.
func TestContactsCaptured(t *testing.T) {
	d, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	pr := d.Entities["contacts"].Operations["putPrivateRestrictedCall"]
	if !pr.Confirmed || !strings.Contains(pr.BodyExample, `"contactType":"blockPrivateRestricted"`) {
		t.Errorf("putPrivateRestrictedCall must be confirmed with contactType blockPrivateRestricted: confirmed=%v ex=%s", pr.Confirmed, pr.BodyExample)
	}
	for _, en := range []string{"contacts", "calls_and_texts"} {
		a := d.Entities[en].Operations["addContactToTheList"]
		if !a.Confirmed || strings.Contains(a.BodyExample, `"TRUSTED"`) || !strings.Contains(a.BodyExample, `"contact"`) {
			t.Errorf("%s.addContactToTheList must be confirmed with a lowercase contactType (not TRUSTED): confirmed=%v ex=%s", en, a.Confirmed, a.BodyExample)
		}
	}
}

// TestAppLimitsCaptured locks in the app-limit + category-block ops captured live via eCapture
// and CLI (2026-08-28): createAppLimit days are lowercase 3-letter (mon..sun), deleteAppLimit is
// by appLimitsId query, and setCFCategories carries the real {categories:[{id,name,subCategories}]}
// request shape (not the bloated view-model). RED against the pre-capture descriptor.
func TestAppLimitsCaptured(t *testing.T) {
	d, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	for _, en := range []string{"content_filter", "schedules"} {
		ca := d.Entities[en].Operations["createAppLimit"]
		if !ca.Confirmed || !strings.Contains(ca.BodyExample, `"mon"`) || strings.Contains(ca.BodyExample, `"Mon"`) {
			t.Errorf("%s.createAppLimit must be confirmed with lowercase days (mon, not Mon): confirmed=%v ex=%s", en, ca.Confirmed, ca.BodyExample)
		}
		if !d.Entities[en].Operations["deleteAppLimit"].Confirmed {
			t.Errorf("%s.deleteAppLimit must be confirmed (verified live)", en)
		}
	}
	sc := d.Entities["content_filter"].Operations["setCFCategories"]
	if !sc.Confirmed || !strings.Contains(sc.BodyExample, "subCategories") || strings.Contains(sc.BodyExample, "isLoading") {
		t.Errorf("setCFCategories must be confirmed with the real request shape (no isLoading view-model field): confirmed=%v ex=%s", sc.Confirmed, sc.BodyExample)
	}
}

// TestSharingConfigCaptured locks in updateLocationSharingSettingConfig, verified live via CLI
// round-trip (2026-08-28, 200): the body carries locationSharingConfig.defaultLocationSharing
// AND the current settings.locationSharingSettings.sharingList (the pre-capture body had neither
// — just settings.sharingList and no config), and eventType=allProfiles is baked. RED against it.
func TestSharingConfigCaptured(t *testing.T) {
	d, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	o := d.Entities["location"].Operations["updateLocationSharingSettingConfig"]
	if !o.Confirmed {
		t.Error("updateLocationSharingSettingConfig must be confirmed (verified live)")
	}
	if !strings.Contains(o.BodyExample, "locationSharingConfig") || !strings.Contains(o.BodyExample, "locationSharingSettings") {
		t.Errorf("body must carry locationSharingConfig + locationSharingSettings: %s", o.BodyExample)
	}
	if !strings.Contains(o.Path, "eventType=allProfiles") {
		t.Errorf("path must bake eventType=allProfiles: %s", o.Path)
	}
}

// TestGeofenceDeleteConfirmed: location.deleteGeofenceSettings was verified live (2026-08-28,
// 200) — a saved location was created via createDeviceGeofenceSettings and deleted by
// eventId+eventType query. RED against the pre-verification descriptor (confirmed was false).
func TestGeofenceDeleteConfirmed(t *testing.T) {
	d, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	o := d.Entities["location"].Operations["deleteGeofenceSettings"]
	if !o.Confirmed {
		t.Error("location.deleteGeofenceSettings must be confirmed (verified live)")
	}
	if len(o.Query) == 0 {
		t.Error("deleteGeofenceSettings must declare its eventId/eventType query params")
	}
}

// TestEmergencyAddCaptured: emergency_contacts.addEmergencyContactsToProfile verified live
// (2026-08-28, added a family member via CLI/app then removed). Body is {contacts:[{userProfileId}]}
// only — the pre-capture body had phantom phone/name fields. RED against the pre-capture descriptor.
func TestEmergencyAddCaptured(t *testing.T) {
	d, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	o := d.Entities["emergency_contacts"].Operations["addEmergencyContactsToProfile"]
	if !o.Confirmed || !strings.Contains(o.BodyExample, "userProfileId") || strings.Contains(o.BodyExample, "phone") || strings.Contains(o.BodyExample, "Grandma") {
		t.Errorf("addEmergencyContactsToProfile must be confirmed with a userProfileId-only body (no phone/name): confirmed=%v ex=%s", o.Confirmed, o.BodyExample)
	}
}

// TestGeofenceUpdateCaptured: geofence.updateDeviceGeofenceSettings body captured from the
// app's successful edit PUT (2026-08-28) — it requires the server-assigned geofenceId in
// deviceGeofenceConfigList (a create omits it) and a geofenceType enum. RED against the
// pre-capture descriptor (which lacked geofenceId and was unconfirmed).
func TestGeofenceUpdateCaptured(t *testing.T) {
	d, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	o := d.Entities["geofence"].Operations["updateDeviceGeofenceSettings"]
	if !o.Confirmed || !strings.Contains(o.BodyExample, "geofenceId") || !strings.Contains(o.BodyExample, "geofenceType") {
		t.Errorf("updateDeviceGeofenceSettings must be confirmed and carry geofenceId + geofenceType: confirmed=%v ex=%s", o.Confirmed, o.BodyExample)
	}
}

// TestTrustedModeConfirmed: contacts.updateTrustedContacts verified live (2026-08-28, 200) —
// the trusted-contacts-only toggle sends {settingId:8000, settingValue:0|1}. The body was already
// correct in the descriptor; this asserts it is now marked confirmed.
func TestTrustedModeConfirmed(t *testing.T) {
	d, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	o := d.Entities["contacts"].Operations["updateTrustedContacts"]
	if !o.Confirmed || !strings.Contains(o.BodyExample, "settingId") {
		t.Errorf("updateTrustedContacts must be confirmed with a settingId body: confirmed=%v ex=%s", o.Confirmed, o.BodyExample)
	}
}

// TestScheduleAliasesFixed: the schedules-entity aliases of scheduled-alert ops share the route
// with the confirmed schedule_alert ops, so they must carry the same corrected body and be
// confirmed (not the old schedule_alert/VSF body). RED against the pre-fix descriptor.
func TestScheduleAliasesFixed(t *testing.T) {
	d, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	o := d.Entities["schedules"].Operations["postScheduleAlert"]
	if !o.Confirmed || !strings.Contains(o.BodyExample, `"locationAlert"`) || strings.Contains(o.BodyExample, "schedule_alert") {
		t.Errorf("schedules.postScheduleAlert alias must be confirmed with the corrected locationAlert body: confirmed=%v ex=%s", o.Confirmed, o.BodyExample)
	}
}

// TestUpdateDeleteOpsConfirmed: updateAppLimit, updateScheduledAlert, and deleteContactFromTheList
// (each on both route-sharing entities) verified live via CLI (2026-08-28, create->update/delete 200).
func TestUpdateDeleteOpsConfirmed(t *testing.T) {
	d, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	check := func(en, op string, want string) {
		o := d.Entities[en].Operations[op]
		if !o.Confirmed {
			t.Errorf("%s.%s must be confirmed (verified live)", en, op)
		}
		if want != "" && !strings.Contains(o.BodyExample, want) {
			t.Errorf("%s.%s body missing %q: %s", en, op, want, o.BodyExample)
		}
	}
	check("content_filter", "updateAppLimit", `"mon"`)
	check("schedules", "updateAppLimit", `"mon"`)
	check("schedule_alert", "updateScheduledAlert", `"locationAlert"`)
	check("schedules", "updateScheduledAlert", `"locationAlert"`)
	check("contacts", "deleteContactFromTheList", "")
	check("calls_and_texts", "deleteContactFromTheList", "")
}

// TestProfileNameAndUserSettingsConfirmed locks in two settings ops verified live 2026-08-29:
// account.updateProfileName (CLI rename round-trip, minimal {"profileName"} body) and
// user_setting.updateUserSettings (KMSI toggle; app_uuid is the caller's own session uuid).
func TestProfileNameAndUserSettingsConfirmed(t *testing.T) {
	d, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	pn := d.Entities["account"].Operations["updateProfileName"]
	if !pn.Confirmed {
		t.Error("account.updateProfileName must be confirmed (verified live)")
	}
	if !strings.Contains(pn.BodyExample, `"profileName"`) {
		t.Errorf("updateProfileName body must carry profileName: %s", pn.BodyExample)
	}
	us := d.Entities["user_setting"].Operations["updateUserSettings"]
	if !us.Confirmed {
		t.Error("user_setting.updateUserSettings must be confirmed (verified live)")
	}
	for _, want := range []string{`"kmsiEnabled"`, `"app_uuid"`, `"triggeredBy"`} {
		if !strings.Contains(us.BodyExample, want) {
			t.Errorf("updateUserSettings body missing %s: %s", want, us.BodyExample)
		}
	}
	// The app_uuid in this op is the caller's own session uuid, so `call` injects it; the
	// identity/pairing token-exchange ops carry a child/device uuid and must NOT be flagged.
	if !us.InjectCallerAppUUID {
		t.Error("updateUserSettings must set InjectCallerAppUUID (app_uuid is the caller's session)")
	}
	for _, en := range []struct{ e, o string }{
		{"identity", "childRefreshToken"}, {"identity", "getChildDeviceAccessToken"}, {"pairing", "getIdTokenUsingOtp"},
	} {
		if d.Entities[en.e].Operations[en.o].InjectCallerAppUUID {
			t.Errorf("%s.%s must NOT set InjectCallerAppUUID (carries a child/device uuid)", en.e, en.o)
		}
	}
}

// TestSettingsGapsConfirmed locks in four guardian-scoped settings ops an adversarial
// reachability audit surfaced and that were then verified live 2026-08-29 (idempotent 200s):
// account family-name/timezone, account profile edit, notifications mark-all-read, and report
// settings (whose body is a flat settings:{id:bool} map, corrected from the guessed keys).
func TestSettingsGapsConfirmed(t *testing.T) {
	d, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	must := func(en, op string, wants ...string) {
		o := d.Entities[en].Operations[op]
		if !o.Confirmed {
			t.Errorf("%s.%s must be confirmed (verified live)", en, op)
		}
		for _, w := range wants {
			if !strings.Contains(o.BodyExample, w) {
				t.Errorf("%s.%s body missing %q: %s", en, op, w, o.BodyExample)
			}
		}
	}
	must("account", "updateFamilyNameOrTimeZone", `"familyName"`, `"timezone"`)
	must("account", "updateProfile", `"profileName"`)
	must("notifications", "markAllRead")
	must("notifications", "updateReportSettings", `"settings"`, `"weeklySummaryEmailV2"`)
	// The prerequisite reads exercised live to verify the writes are confirmed too, so
	// `describe --json` does not show a verified write depending on an "unverified" read.
	for _, op := range []string{"getReportSettings2", "getNotificationCount"} {
		if !d.Entities["notifications"].Operations[op].Confirmed {
			t.Errorf("notifications.%s must be confirmed (exercised live)", op)
		}
	}
}

// TestDeviceGatedConfirmed locks in the device-gated mutations unblocked by pairing a child
// device and verified live 2026-09-07: internet pause/resume and website allow/block/remove.
// pauseInternet's pauseSchedule is the display timing with spaces replaced by underscores.
func TestDeviceGatedConfirmed(t *testing.T) {
	d, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	pi := d.Entities["pause_internet"].Operations["pauseInternet"]
	if !pi.Confirmed || !strings.Contains(pi.BodyExample, "30_minutes") {
		t.Errorf("pauseInternet must be confirmed with the underscore schedule value: %v %s", pi.Confirmed, pi.BodyExample)
	}
	for _, op := range []string{"unPauseInternet"} {
		if !d.Entities["pause_internet"].Operations[op].Confirmed {
			t.Errorf("pause_internet.%s must be confirmed (verified live)", op)
		}
	}
	for _, op := range []string{"postWebsite", "postWebsites", "deleteWebsite"} {
		if !d.Entities["website"].Operations[op].Confirmed {
			t.Errorf("website.%s must be confirmed (verified live)", op)
		}
	}
	if !strings.Contains(d.Entities["website"].Operations["postWebsite"].BodyExample, `"status"`) {
		t.Error("postWebsite body must document the status field (a=allow, b=block)")
	}
	// The prerequisite reads exercised to verify/reverse these writes are confirmed too.
	if !d.Entities["pause_internet"].Operations["getDevices"].Confirmed {
		t.Error("pause_internet.getDevices must be confirmed (exercised live)")
	}
	if !d.Entities["web_and_apps"].Operations["getWebsites2"].Confirmed {
		t.Error("web_and_apps.getWebsites2 must be confirmed (exercised live)")
	}
}

// TestDeadEndsDisabled locks in the confirmed dead-end entities (products/devices this
// account lacks, or child-device-originated telemetry): every op unavailable, entity fully
// hidden. Disabled, not deleted — the ops still exist. See docs/unavailable-endpoints.md.
func TestDeadEndsDisabled(t *testing.T) {
	d, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	// Fully-hidden entities: every op a confirmed dead end (Gizmo group-chat/video, child-device
	// telemetry) with no callable op, so `entities` hides them.
	for _, en := range []string{"messaging", "video_calling", "installed_apps"} {
		e := d.Entities[en]
		if e.AvailableOps() != 0 {
			t.Errorf("%s must be fully unavailable, got %d available ops", en, e.AvailableOps())
		}
	}
	// Mixed entities: some ops disabled, at least one still callable, so they stay listed.
	// tamper keeps the parent-facing putTamperInstructions (same route as dashboard's); the
	// product entities keep the route-shared/general ops (purchase link, invite resend, etc.).
	for _, m := range []struct{ en, disabled, available string }{
		{"tamper", "postBatteryUsage", "putTamperInstructions"},
		{"pet_tracker", "submitPetLiveTracker", "getPurchaseLink"},
		{"wearable", "watchAuth", "resendInvite"},
	} {
		e := d.Entities[m.en]
		if e.Operations[m.disabled].Available() {
			t.Errorf("%s.%s must be disabled (product/child dead end)", m.en, m.disabled)
		}
		if !e.Operations[m.available].Available() {
			t.Errorf("%s.%s must stay available (route reachable elsewhere / parent-facing)", m.en, m.available)
		}
		if e.AvailableOps() == 0 {
			t.Errorf("%s must keep at least one callable op", m.en)
		}
	}
	// Driving Insights settings, reachable once DI is enabled on the member device, are confirmed.
	for _, op := range []string{"putSettings", "putSpeedAlertLimit"} {
		if !d.Entities["driving_insights"].Operations[op].Confirmed {
			t.Errorf("driving_insights.%s must be confirmed (verified live)", op)
		}
	}
}

// TestNoUnavailableSharesRouteWithAvailable enforces the invariant that fixes the Codex
// finding class (tamper.putTamperInstructions, gizmo/pet/wearable aliases): if any op on a
// (method,path) route is callable, none on that route may be marked unavailable — otherwise
// the CLI would disable functionality reachable via a sibling op.
func TestNoUnavailableSharesRouteWithAvailable(t *testing.T) {
	d, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	type route struct{ method, path string }
	avail := map[route]string{}
	unavail := map[route]string{}
	for en, e := range d.Entities {
		for _, grp := range []map[string]Operation{e.Operations, e.Actions} {
			for op, o := range grp {
				r := route{o.Method, o.Path}
				if o.Available() {
					avail[r] = en + "." + op
				} else {
					unavail[r] = en + "." + op
				}
			}
		}
	}
	for r, u := range unavail {
		if a, ok := avail[r]; ok {
			t.Errorf("%s is unavailable but shares route %s %s with the available %s — disable both or neither", u, r.method, r.path, a)
		}
	}
}

// TestEnabledFeatureVerbs: roadside_assistance.updateRsaMemberAccess verified live (2026-09-07)
// when enabling RSA; and the family_line entity documents its SPC-token auth requirement.
func TestEnabledFeatureVerbs(t *testing.T) {
	d, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	rsa := d.Entities["roadside_assistance"].Operations["updateRsaMemberAccess"]
	if !rsa.Confirmed || !strings.Contains(rsa.BodyExample, "enableRSAFlag") {
		t.Errorf("updateRsaMemberAccess must be confirmed with enableRSAFlag: %v %s", rsa.Confirmed, rsa.BodyExample)
	}
	if !strings.Contains(d.Entities["family_line"].Summary, "SPC token") {
		t.Error("family_line summary must document the SPC-token auth requirement")
	}
}

// TestFamilyLineVerifiedRoutes pins the wire-verified (2026-09-07, decrypted HPACK)
// finding that getProvisioningStatus (fl/status) and traceSdkResponse (fl/trace)
// authenticate with the plain id_token + x-fp-identifier-target-serviceid, NOT an SPC
// token — so they must be confirmed and declare the target-serviceid header (so
// `call` injects --service-id). The blanket "all family_line ⇒ SPC" assertion was stale.
func TestFamilyLineVerifiedRoutes(t *testing.T) {
	d, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	fl := d.Entities["family_line"]
	for _, name := range []string{"getProvisioningStatus", "traceSdkResponse"} {
		op, ok := fl.Operations[name]
		if !ok {
			t.Fatalf("family_line.%s missing", name)
		}
		if !op.Confirmed {
			t.Errorf("family_line.%s must be confirmed (verified plain-id_token route)", name)
		}
		hasSvc := false
		for _, h := range op.Headers {
			if strings.Contains(h, "serviceid") {
				hasSvc = true
			}
		}
		if !hasSvc {
			t.Errorf("family_line.%s must declare x-fp-identifier-target-serviceid so --service-id is injected; headers=%v", name, op.Headers)
		}
	}
	// The summary must still document SPC (for the management ops) AND the plain-id_token exception.
	sum := fl.Summary
	if !strings.Contains(sum, "SPC token") || !strings.Contains(sum, "parent id_token") {
		t.Error("family_line summary must scope SPC to management ops and record the plain-id_token routes")
	}
}

func TestDefaultLoads(t *testing.T) {
	d, err := Default()
	if err != nil {
		t.Fatalf("Default() error: %v", err)
	}
	if d.BaseURL != "https://api.prd.vsf.aws.vz-connect.com" {
		t.Errorf("unexpected base_url: %q", d.BaseURL)
	}
	if d.Auth.ClientID == "" {
		t.Error("auth.client_id must not be empty")
	}
	// The whole point of the CLI is broad coverage; guard against an accidentally
	// gutted descriptor.
	if got := len(d.Entities); got < 8 {
		t.Errorf("expected the data model to cover >=8 entities, got %d", got)
	}
	if _, ok := d.Entity("content_filter"); !ok {
		t.Error("expected a content_filter entity")
	}
	// The harvested surface is large; make sure it stayed comprehensive.
	if got := len(d.Entities); got < 40 {
		t.Errorf("expected the harvested data model to cover >=40 entities, got %d", got)
	}
}

func TestParseRejectsEmptyBaseURL(t *testing.T) {
	_, err := Parse([]byte(`{"name":"x","entities":{"a":{}}}`))
	if err == nil {
		t.Fatal("expected an error for empty base_url")
	}
}

func TestParseRejectsNoEntities(t *testing.T) {
	_, err := Parse([]byte(`{"name":"x","base_url":"https://h"}`))
	if err == nil {
		t.Fatal("expected an error for no entities")
	}
}

// Parse rejects a descriptor whose op names a required_query param that is not in its query
// list: `describe` lists query, and `call` demands required_query, so a required name absent
// from query would be undocumented — a descriptor bug caught at load rather than confusing a
// caller at runtime.
func TestParseRejectsRequiredQueryNotInQuery(t *testing.T) {
	js := `{"name":"x","base_url":"https://h","entities":{"a":{"id_field":"","operations":` +
		`{"getX":{"method":"GET","path":"/x","query":["startDate"],"required_query":["endDate"]}}}}}`
	_, err := Parse([]byte(js))
	if err == nil || !strings.Contains(err.Error(), "required_query") {
		t.Fatalf("want a required_query-not-in-query error, got %v", err)
	}
	// A required_query that IS a subset of query loads fine.
	ok := `{"name":"x","base_url":"https://h","entities":{"a":{"id_field":"","operations":` +
		`{"getX":{"method":"GET","path":"/x","query":["startDate","endDate"],"required_query":["startDate"]}}}}}`
	if _, err := Parse([]byte(ok)); err != nil {
		t.Fatalf("valid required_query subset should load: %v", err)
	}
}

// The calls_and_texts activity family's required_query is pinned to what was verified against
// the live API (each op 400s "<field> is empty" for a missing required param), so it is not
// silently dropped or widened to unverified params. Notably getTopContactListByActivityV7's
// profileId/deviceId are NOT required (dates-only returns 404 not-found, not a 400), so they
// must stay out of required_query — a good guard against guessing "ids are always required".
func TestCallTextActivityRequiredQuery(t *testing.T) {
	d, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	e, _ := d.Entity("calls_and_texts")
	want := map[string][]string{
		"getCallAndTextActivityListV7":                {"startDate", "endDate"},
		"getCallAndTextProfileSummaryListV7":          {"startDate", "endDate"},
		"getCallAndTextSpecificContactActivityListV7": {"otherPartyMdn", "startDate", "endDate"},
		"getTopContactListByActivityV7":               {"startDate", "endDate"},
	}
	for opName, wantReq := range want {
		op := e.Operations[opName]
		got := map[string]bool{}
		for _, r := range op.RequiredQuery {
			got[r] = true
		}
		if len(op.RequiredQuery) != len(wantReq) {
			t.Errorf("%s required_query = %v, want %v", opName, op.RequiredQuery, wantReq)
			continue
		}
		for _, r := range wantReq {
			if !got[r] {
				t.Errorf("%s required_query = %v, missing %q (want %v)", opName, op.RequiredQuery, r, wantReq)
			}
		}
		// Documented-optional params must never be marked required.
		if got["betaProviders"] || got["summaryOnly"] || got["profileId"] || got["deviceId"] {
			t.Errorf("%s marks an optional param required: %v", opName, op.RequiredQuery)
		}
	}
}

func TestNamesAreSorted(t *testing.T) {
	d, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	names := d.EntityNames()
	for i := 1; i < len(names); i++ {
		if names[i-1] > names[i] {
			t.Fatalf("EntityNames not sorted: %v", names)
		}
	}
}

// The descriptor must preserve every statically-known header name for an op — Codex #20
// flagged that activity_tracking getDailyActivities dropped its required timezone /
// schedule-type headers, leaving the single source of truth incomplete.
func TestDeclaredHeadersPreserved(t *testing.T) {
	d, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	e, ok := d.Entity("activity_tracking")
	if !ok {
		t.Fatal("no activity_tracking entity")
	}
	op, ok := e.Operations["getDailyActivities"]
	if !ok {
		t.Fatal("no getDailyActivities op")
	}
	want := map[string]bool{"timezone": false, "schedule-type": false, "x-fp-identifier-target-serviceid": false}
	for _, h := range op.Headers {
		if _, ok := want[h]; ok {
			want[h] = true
		}
	}
	for h, seen := range want {
		if !seen {
			t.Errorf("getDailyActivities is missing the declared header %q (have %v)", h, op.Headers)
		}
	}
}

// The catastrophic, irreversible ops must stay marked Destructive so `call`'s --confirm
// guard covers them (Codex flagged pairing.deleteMediaBackupEntries slipping through).
func TestDestructiveOpsMarked(t *testing.T) {
	d, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	want := [][2]string{
		{"account", "deleteProfile"}, {"account", "deleteMyself"},
		{"account", "selfRemoveProfile"}, {"account", "deleteDevice"},
		{"subscription", "cancelSubscription"}, {"pairing", "unlinkGizmoAccount"},
		{"pairing", "deleteMediaBackupEntries"},
		{"messaging", "deleteMessages"}, {"messaging", "deleteGroupChat"},
		{"messaging", "clearAllGroupChatMessages"}, {"messaging", "deleteGroupMember"},
		{"contacts", "bulkDeleteContacts"},
		{"family_line", "deProvisionFamilyLine"}, {"family_line", "removeUserFromFamilyLine"},
		{"professional_monitoring", "deactivateProfile"},
	}
	for _, w := range want {
		e, ok := d.Entity(w[0])
		if !ok {
			t.Errorf("no entity %q", w[0])
			continue
		}
		op, ok := e.Operations[w[1]]
		if !ok {
			t.Errorf("no op %s.%s", w[0], w[1])
			continue
		}
		if !op.Destructive {
			t.Errorf("%s.%s must be marked Destructive (irreversible)", w[0], w[1])
		}
	}
}

// Every op sharing a wire route (method+path) with a destructive op must ALSO be
// destructive — the backend sees only the route + body, so an unmarked alias would let a
// caller reach the destructive route without --confirm (Codex: professional_monitoring
// reactivateProfile shares deactivateProfile's PATCH route).
func TestDestructiveRoutesFullyGated(t *testing.T) {
	d, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	type route struct{ m, p string }
	byRoute := map[route][]Operation{}
	names := map[route][]string{}
	for en, e := range d.Entities {
		add := func(opn string, op Operation) {
			r := route{op.Method, op.Path}
			byRoute[r] = append(byRoute[r], op)
			names[r] = append(names[r], en+"."+opn)
		}
		for opn, op := range e.Operations {
			add(opn, op)
		}
		for opn, op := range e.Actions {
			add(opn, op)
		}
	}
	for r, ops := range byRoute {
		anyDest := false
		for _, op := range ops {
			anyDest = anyDest || op.Destructive
		}
		if !anyDest {
			continue
		}
		for i, op := range ops {
			if !op.Destructive {
				t.Errorf("%s shares destructive route %s %s but is not gated", names[r][i], r.m, r.p)
			}
		}
	}
}

// Body examples come from each op's own model (not a route-sibling), and multipart ops
// carry no JSON example (Codex #24).
func TestBodyExampleAccuracy(t *testing.T) {
	d, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	op := func(en, name string) Operation {
		e, _ := d.Entity(en)
		if o, ok := e.Operations[name]; ok {
			return o
		}
		return e.Actions[name]
	}
	if ex := op("schedules", "putScreenTimeData").BodyExample; !strings.Contains(ex, "weeklyLimits") {
		t.Errorf("putScreenTimeData example = %q, want weeklyLimits", ex)
	}
	if ex := op("schedules", "acceptDeclineAskTimeRequest").BodyExample; !strings.Contains(ex, "additionalTimeAction") {
		t.Errorf("acceptDeclineAskTimeRequest example = %q, want the decision field", ex)
	}
	if o := op("messaging", "sendGroupMediaMessage"); o.BodyExample != "" || !o.Multipart {
		t.Errorf("sendGroupMediaMessage must be multipart with no JSON example: multipart=%v ex=%q", o.Multipart, o.BodyExample)
	}
}

// TestLiveVerifiedMutations locks in the mutation bodies verified against the live app via
// eCapture (2026-08-26): the app-block/subcategory POST and the schedules PUT. These assertions
// fail against the pre-verification descriptor (blockApp/updateSubcategory were not confirmed;
// putSchedule carried the incomplete {"id":"12345"} stub) — RED without the capture work.
func TestLiveVerifiedMutations(t *testing.T) {
	d, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	op := func(en, name string) Operation {
		e, _ := d.Entity(en)
		if o, ok := e.Operations[name]; ok {
			return o
		}
		return e.Actions[name]
	}
	// app-block / content subcategory: POST /v8/subcategory, body {"subcategory":{...}}.
	if o := op("app_block", "blockApp"); !o.Confirmed ||
		!strings.Contains(o.BodyExample, `"subcategory"`) || !strings.Contains(o.BodyExample, `"categoryShortName"`) {
		t.Errorf("app_block.blockApp must be confirmed with the subcategory body: confirmed=%v ex=%q", o.Confirmed, o.BodyExample)
	}
	if o := op("content_filter", "updateSubcategory"); !o.Confirmed || !strings.Contains(o.BodyExample, `"subcategory"`) {
		t.Errorf("content_filter.updateSubcategory must be confirmed with a subcategory body: confirmed=%v ex=%q", o.Confirmed, o.BodyExample)
	}
	// schedules PUT: the verified body is a schedules[] array of real fields, not the {"id"} stub.
	put := op("schedules", "putSchedule")
	if !put.Confirmed {
		t.Error("schedules.putSchedule must be confirmed (verified live)")
	}
	for _, want := range []string{`"schedules"`, "scheduleType", "alertOn", "blockContent", "startTime"} {
		if !strings.Contains(put.BodyExample, want) {
			t.Errorf("schedules.putSchedule body_example missing %q: %q", want, put.BodyExample)
		}
	}
	if strings.Contains(put.BodyExample, `"id":"12345"`) {
		t.Errorf("schedules.putSchedule still carries the incomplete stub: %q", put.BodyExample)
	}
	// postSchedule shares the enriched schedules[] schema (create omits scheduleId).
	if ex := op("schedules", "postSchedule").BodyExample; !strings.Contains(ex, "scheduleType") || strings.Contains(ex, `"id":"12345"`) {
		t.Errorf("schedules.postSchedule body_example not enriched: %q", ex)
	}
}

// TestWireVerifiedOpsConfirmed pins the ops whose exact requests were decrypted off the
// wire (2026-09-07/08, Android-17 TLS 1.3 → HPACK) and are therefore marked confirmed.
// Each must stay confirmed; the VSF read/write ops among them must declare the
// x-fp-identifier-target-serviceid header so `call` injects --service-id.
func TestWireVerifiedOpsConfirmed(t *testing.T) {
	d, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	wire := [][2]string{
		{"accessibility_pin", "getAgeCaptureNotification"},
		{"account", "getAccountDetails"},
		{"account", "getProfileImage"},
		{"calendar_sync", "getSchoolSelection"},
		{"calls_and_texts", "getCallAndTextActivityListV7"},
		{"config", "getConfigData"},
		{"config", "getNonSecureConfigData"},
		{"config", "getServiceKeys"},
		{"contacts", "getAllContactsRequest"},
		{"content_filter", "getParentalControls"},
		{"cross_sell", "getCrossSellRecommendations"},
		{"dashboard", "getSubscriptions"},
		{"dashboard", "getViewBanner"},
		{"driving_insights", "getSettings"},
		{"driving_insights", "getTripSummary"},
		{"family_line", "getProvisioningStatus"},
		{"family_line", "traceSdkResponse"},
		{"feature_permissions", "getManagedUserProfiles"},
		{"geofence", "getSavedLocations"},
		{"location", "getDashboardDetails"},
		{"location", "getHistoryStatus"},
		{"location", "getPickMeUpStatus"},
		{"location", "getWithWhomIamSharingLocation"},
		{"location", "postLocationTamper"},
		{"notifications", "getNotificationCount"},
		{"pairing", "getConsent"},
		{"pairing", "getDeviceShadowDetails"},
		{"pairing", "gizmoImportEligibility"},
		{"pause_internet", "getDevices"},
		{"pause_internet", "pauseInternet"},
		{"pause_internet", "unPauseInternet"},
		{"pet_tracker", "getAllAvailableEmergencyContacts"},
		{"professional_monitoring", "getSubscriberSetupInfo"},
		{"profile", "getTopApps"},
		{"pubnub", "getPubNubConfig"},
		{"pubnub", "putDeviceToken"},
		{"restricted_usage", "getAllTheLimits"},
		{"roadside_assistance", "getRescueInfo"},
		{"schedules", "getSchedules"},
		{"schedules", "getScreenTimeData"},
		{"services_hub", "getEligibleServices"},
		{"setup_wizard", "getUserTasks"},
		{"sos", "getWatchMeSoSSessionInfo"},
		{"sos", "watchMeSosAlerts"},
		{"user_setting", "updateUserSettings"},
		{"web_and_apps", "getInsights"},
	}
	for _, w := range wire {
		e, ok := d.Entity(w[0])
		if !ok {
			t.Errorf("entity %q missing", w[0])
			continue
		}
		op, ok := e.Operations[w[1]]
		if !ok {
			if op, ok = e.Actions[w[1]]; !ok {
				t.Errorf("%s.%s missing", w[0], w[1])
				continue
			}
		}
		if !op.Confirmed {
			t.Errorf("%s.%s must be confirmed (decrypted off the wire)", w[0], w[1])
		}
	}
}
