# Verizon Family / SafePath — API catalog

Harvested by static analysis of the signed APK **v8.101.30** (build 810100030; decompiled Retrofit interfaces). Machine-readable source: [`vsf-endpoints.json`](vsf-endpoints.json); the CLI's embedded descriptor is [`internal/descriptor/verizon_family.json`](../internal/descriptor/verizon_family.json).

**489 operations** across **110 Retrofit interfaces**, grouped below by feature — this is the raw Retrofit-method harvest (methods repeat across interfaces/features); the CLI's curated descriptor de-dupes it to **459 operations / 59 entities**. HTTP method + path are confirmed present as string constants; request/response bodies were statically inferred from the decompiled model classes, and a subset has since been confirmed live by an **eCapture** (eBPF) capture — request bodies byte-diffed against real app traffic and accepted by production (method: [`PROCESS.md`](PROCESS.md) §5–§7).


## accessibilityProtection (7)

| Method | HTTP | Path | Headers | Query |
|---|---|---|---|---|
| retrievePin | GET | `/frisco/parental-control/v5/accessibility/pin/retrieve` | `x-fp-identifier-target-serviceid` | `newPin` |
| validatePin | GET | `/frisco/parental-control/v5/accessibility/pin/validate` | `x-fp-identifier-target-serviceid` | `pin` |
| getAgeCaptureNotification | GET | `/frisco/parental-control/v5/ageCaptureNotif` | `x-fp-identifier-target-serviceid` | — |
| getDeviceShadowDetails | GET | `vsf/tamper/v6/view/getDeviceShadowDetails` | `x-fp-identifier-target-serviceid` | `deviceID`, `featureIds`, `retrieveLevel` |
| postAgeCaptureNotification | POST | `/frisco/parental-control/v5/ageCaptureNotif` | `x-fp-identifier-target-serviceid` | — |
| sendPin | POST | `frisco/commsplatform/v5/sms-push/notification` | `x-fp-identifier-target-serviceid` | — |
| putAgeCaptureNotification | PUT | `/frisco/parental-control/v5/ageCaptureNotif` | `x-fp-identifier-target-serviceid` | — |

## accessibilityProtection (OUT-OF-GROUP) (3)

| Method | HTTP | Path | Headers | Query |
|---|---|---|---|---|
| getAgeCaptureNotification | GET | `/frisco/parental-control/v5/ageCaptureNotif` | `x-fp-identifier-target-serviceid` | — |
| postAgeCaptureNotification | POST | `/frisco/parental-control/v5/ageCaptureNotif` | `x-fp-identifier-target-serviceid` | — |
| putAgeCaptureNotification | PUT | `/frisco/parental-control/v5/ageCaptureNotif` | `x-fp-identifier-target-serviceid` | — |

## accounts-profiles-hub / accounts (16)

| Method | HTTP | Path | Headers | Query |
|---|---|---|---|---|
| selfRemoveProfile | DELETE | `/account/fam/userprofile-management/v5/accounts/userprofiles/self` | `x-fp-identifier-target-serviceid` | — |
| deleteMyself | DELETE | `/account/fam/userprofile-management/v5/accounts/userprofiles/services` | `x-fp-identifier-target-serviceid` | — |
| deleteProfile | DELETE | `/fam/userprofile-management/v5/accounts/userprofiles` | `x-fp-identifier-target-serviceid` | — |
| deleteDevice | DELETE | `/fam/userprofile-management/v5/accounts/userprofiles/devices` | `x-fp-identifier-target-serviceid` | — |
| getAccountDetails | GET | `account/fam/userprofile-management/v8/accounts/userprofiles` | `(@HeaderMap dynamic)` | — |
| getProfileImage | GET | `account/frisco/userprofile-management/v6/accounts/userprofiles/images` | `(@HeaderMap dynamic)` | — |
| attestDob | PATCH | `/account/fam/userprofile-management/v5/userprofiles/age/attestation` | `x-fp-identifier-target-serviceid` | — |
| updateFamilyNameOrTimeZone | PATCH | `/fam/userprofile-management/v5/accounts` | `x-fp-identifier-target-serviceid`, `x-pending-activation` | — |
| updateProfileName | PATCH | `/fam/userprofile-management/v5/accounts/userprofiles` | `x-fp-identifier-target-serviceid`, `x-pending-activation` | — |
| updateDeviceName | PATCH | `/fam/userprofile-management/v5/accounts/userprofiles/devices` | `x-fp-identifier-target-serviceid` | — |
| updateProfile | PATCH | `/fam/userprofile-management/v5/userprofiles` | `x-fp-identifier-target-serviceid` | — |
| addOnboardingDevice | POST | `/account/fam/userprofile-management/v5/onboarding/device` | — | — |
| choosePlan | POST | `/fam/userprofile-management/v5/accounts/subscriptions/{planType}` | — | — |
| addDevice | POST | `/fam/userprofile-management/v5/accounts/userprofiles/devices` | `x-fp-identifier-target-serviceid` | — |
| updatePlan | PUT | `/fam/userprofile-management/v5/accounts/subscriptions/{planType}` | `x-fp-identifier-target-serviceid` | — |
| updateProfileImage | PUT | `/fam/userprofile-management/v5/accounts/userprofiles/images` | `x-fp-identifier-target-serviceid` | — |

## accounts-profiles-hub / accounts-usersetting (1)

| Method | HTTP | Path | Headers | Query |
|---|---|---|---|---|
| updateUserSettings | POST | `/frisco/frisco-iam-device-auth/v5/user/auth/setting` | — | — |

## accounts-profiles-hub / config (9)

| Method | HTTP | Path | Headers | Query |
|---|---|---|---|---|
| getAccessConfig | GET | `(dynamic @Url)` | `If-None-Match` | — |
| getDeviceTypeConfig | GET | `(dynamic @Url)` | `If-None-Match` | — |
| getPermissionConfig | GET | `(dynamic @Url)` | `If-None-Match` | — |
| getServiceKeys | GET | `/frisco/key-mgmt/v5/keys` | — | `keys` |
| getConfigData | GET | `auth/frisco/mappcontent/v6/configs` | `(@HeaderMap dynamic)` | `(@QueryMap dynamic)` |
| getNonSecureConfigData | GET | `nsauth/frisco/mappcontent/v6/configs/nonSecure` | `(@HeaderMap dynamic)` | `(@QueryMap dynamic)` |
| getConfigData (Call variant) | GET | `sf/v1/prd/mcfg.json` | — | — |
| getCustomMapSatelliteStyle | GET | `sf/v1/prd/satelliteStyle.json` | — | — |
| getCustomMapStreetStyle | GET | `sf/v1/prd/style.json` | — | — |

## accounts-profiles-hub / dashboard (15)

| Method | HTTP | Path | Headers | Query |
|---|---|---|---|---|
| getSubscriptions | GET | `/account/fam/userprofile-management/v5/accounts/userprofiles/subscriptions` | `x-fp-identifier-target-serviceid` | — |
| getEligibility | GET | `/fam/userprofile-management/v5/accounts/features/eligibility` | `x-user-action` | — |
| dashboardFlow | GET | `/vsf/account-management/v1/accounts` | — | — |
| queryEligibleLines | GET | `/vsf/account-management/v1/accounts/lines` | — | — |
| getReportSetting | GET | `/vsf/commsplatform/v5/report-settings?reportCategory=all` | `x-fp-identifier-target-serviceid` | — |
| getReportSettings | GET | `/vsf/commsplatform/v5/report-settings?reportCategory=individual` | `x-fp-identifier-target-serviceid` | — |
| getViewBanner | GET | `/vsf/tamper/v5/view/getBanner` | `x-fp-identifier-target-serviceid` | `profiles`, `hasParentalCtrlPermissions` |
| getViewBannerDetails | GET | `/vsf/tamper/v5/view/getBannerDetails` | `x-fp-identifier-target-serviceid` | `feature`, `profiles`, `hasParentalCtrlPermissions` |
| acknowledgeLuciqPush | POST | `/frisco/commsplatform/v5/pushStatus` | `x-fp-identifier-target-serviceid` | — |
| createProfile | POST | `/vsf/account-management/v1/profiles` | — | — |
| postReportSetting | POST | `/vsf/commsplatform/v5/report-settings?reportCategory=all` | `x-fp-identifier-target-serviceid` | — |
| postReportSettings | POST | `/vsf/commsplatform/v5/report-settings?reportCategory=individual` | `x-fp-identifier-target-serviceid` | — |
| putTamperInstructions | PUT | `/fam/userprofile-management/v5/tamper/instructions` | `x-fp-identifier-target-serviceid` | — |
| putRestrictionsBackOn | PUT | `/frisco/callandtext/v5/911/restrictions` | `x-fp-identifier-target-serviceid` | — |
| sendInvitation | PUT | `/vsf/account-management/v1/profiles/invite` | — | — |

## accounts-profiles-hub / dashboard-tips (1)

| Method | HTTP | Path | Headers | Query |
|---|---|---|---|---|
| getParentingTips | GET | `about/api/smart-family/parenting-digital-world` | — | `offset`, `limit` |

## accounts-profiles-hub / notifications (3)

| Method | HTTP | Path | Headers | Query |
|---|---|---|---|---|
| getNotificationFeed | GET | `/frisco/commsplatform/v5/notifications` | `x-fp-identifier-target-serviceid` | `productType`, `notificationType`, `exclude` |
| getObjectionableWeb | GET | `/frisco/parental-control/v5/noteworthy-details/{eventId}` | `x-fp-identifier-target-serviceid` | `notification-type` |
| getReportSettings | GET | `/vsf/commsplatform/v5/report-settings?reportCategory=individual` | `x-fp-identifier-target-serviceid` | — |

## accounts-profiles-hub / permissions (5)

| Method | HTTP | Path | Headers | Query |
|---|---|---|---|---|
| getManagedUserProfiles | GET | `/account/fam/userprofile-management/v5/userprofiles/connections` | `x-fp-identifier-target-serviceid` | — |
| getParentalControlFeaturePermissions | GET | `/vsf/commsplatform/v5/view/featurePermissions` | `x-fp-identifier-target-serviceid` | `featureGroup` |
| updateUserProfileAccess | PATCH | `/account/fam/userprofile-management/v5/userprofiles/access` | `x-fp-identifier-target-serviceid` | — |
| changeUserProfileRole | PUT | `/account/fam/userprofile-management/v5/accounts/userprofiles/role` | `x-fp-identifier-target-serviceid` | — |
| updatePermissions | PUT | `/account/fam/userprofile-management/v5/userprofiles/permissions` | `x-fp-identifier-target-serviceid` | — |

## accounts-profiles-hub / plusfamily-invites (7)

| Method | HTTP | Path | Headers | Query |
|---|---|---|---|---|
| getAccountLines | GET | `/fam/userprofile-management/v5/accounts/lines` | `x-fp-identifier-target-serviceid` | — |
| getFeaturePermissions | GET | `/fam/userprofile-management/v5/accounts/userprofiles/featurepermissions` | `x-fp-identifier-target-serviceid` | — |
| replaceDevice | PATCH | `/account/fam/userprofile-management/v5/gizmo/device/replace` | `x-fp-identifier-target-serviceid` | — |
| sendStandaloneInvite | POST | `/account/fam/userprofile-management/v5/onboarding/device` | `x-fp-identifier-target-serviceid`, `x-pairing-required` | — |
| sendInvite | POST | `/account/fam/userprofile-management/v6/accounts/userprofiles` | `x-fp-identifier-target-serviceid` | — |
| updateFeaturePermissions | POST | `/fam/userprofile-management/v5/accounts/userprofiles/featurepermissions` | `x-fp-identifier-target-serviceid` | — |
| retryPairing | PUT | `/account/fam/userprofile-management/v5/gizmo/device/pairing` | `x-fp-identifier-target-serviceid` | — |

## accounts-profiles-hub / plusfamily-profiles (1)

| Method | HTTP | Path | Headers | Query |
|---|---|---|---|---|
| reSendInvite | PATCH | `/fam/userprofile-management/v5/userprofiles/services/invites` | `x-fp-identifier-target-serviceid` | — |

## accounts-profiles-hub / profile (1)

| Method | HTTP | Path | Headers | Query |
|---|---|---|---|---|
| getTopApps | GET | `/parental-control/frisco/v5/web-app-activity/topApps` | `x-fp-identifier-target-serviceid` | `date`, `count`, `timezone` |

## accounts-profiles-hub / profiles-avatars (1)

| Method | HTTP | Path | Headers | Query |
|---|---|---|---|---|
| getProfileAvatars | GET | `/parental-control/frisco/v5/profile-avatars` | `x-fp-identifier-target-serviceid` | `resolution` |

## accounts-profiles-hub / pubnub-config (3)

| Method | HTTP | Path | Headers | Query |
|---|---|---|---|---|
| getPubNubConfig | GET | `/frisco/commsplatform/v5/profiles/pubnubConfiguration` | `x-fp-identifier-target-serviceid` | — |
| getPubNubToken | POST | `/frisco/commsplatform/v5/pubnub-token` | `x-fp-identifier-target-serviceid` | — |
| putDeviceToken | PUT | `/frisco/commsplatform/v5/profiles/pushToken` | `x-fp-identifier-target-serviceid` | — |

## accounts-profiles-hub / serviceshub (3)

| Method | HTTP | Path | Headers | Query |
|---|---|---|---|---|
| getTileImages | GET | `servicecatalog/fam/servicescatalog/v1/images` | `x-fp-identifier-target-serviceid` | — |
| getSetupStatus | GET | `servicecatalog/fam/servicescatalog/v5/setupstatus` | `x-fp-identifier-target-serviceid`, `x-fp-identifier-app-uuid`, `If-None-Match` | `tile-id` |
| getEligibleServices | GET | `servicecatalog/fam/servicescatalog/v6/eligibleservices` | `x-fp-identifier-target-serviceid`, `x-fp-identifier-app-uuid` | — |

## advance-activity-tracking (2)

| Method | HTTP | Path | Headers | Query |
|---|---|---|---|---|
| getActivity | GET | `/comms/fam/v1/device/activities` | `x-fp-identifier-target-serviceid`, `If-None-Match`, `timezone`, `schedule-type` | — |
| getDailyActivities | GET | `/comms/fam/v1/device/activities` | `x-fp-identifier-target-serviceid`, `If-None-Match`, `timezone`, `schedule-type` | — |

## ageverification (3)

| Method | HTTP | Path | Headers | Query |
|---|---|---|---|---|
| getSdkLicenseKey | GET | `/frisco/key-mgmt/v5/keys` | `x-fp-identifier-target-serviceid` | `keys` |
| getPublicKey | GET | `/frisco/key-mgmt/v5/mitek/keys` | `x-fp-identifier-target-serviceid` | `user-profile-id` |
| submitVerification | POST | `/account/fam/userprofile-management/v5/userprofiles/age/verify` | `x-fp-identifier-target-serviceid`, `user-profile-id` | `user-profile-id` |

## app-block (1)

| Method | HTTP | Path | Headers | Query |
|---|---|---|---|---|
| blockApp | POST | `/parental-control/frisco/v8/subcategory` | `x-fp-identifier-target-serviceid` | — |

## appblocking (4)

| Method | HTTP | Path | Headers | Query |
|---|---|---|---|---|
| getBlockedApps | GET | `/parental-control/frisco/v8/devices/{deviceId}/appsSync` | `If-None-Match`, `x-fp-identifier-target-serviceid` | — |
| sendAccessibilityStatus | POST | `/parental-control/frisco/v8/accessibility/status` | `x-fp-identifier-target-serviceid` | — |
| sendBlockStatus | POST | `/parental-control/frisco/v8/androidApps/blockStatus` | `x-fp-identifier-target-serviceid` | — |
| blockApp | POST | `/parental-control/frisco/v8/subcategory` | `x-fp-identifier-target-serviceid` | — |

## appmanagement (3)

| Method | HTTP | Path | Headers | Query |
|---|---|---|---|---|
| getInteractionData | GET | `/comms/fam/v1/device/ai/logs` | `x-fp-identifier-target-serviceid` | `logId`, `limit`, `direction` |
| getAppUsages | GET | `/comms/fam/v1/device/app/management/usage` | `x-fp-identifier-target-serviceid`, `timezone`, `usage-type`, `If-None-Match` | — |
| updateAppStatus | POST | `/account/fam/device-management/v5/device/app/management` | `x-fp-identifier-target-serviceid`, `x-fp-identifier-profileid` | — |

## bazaarvoice-pdp-rating (3)

| Method | HTTP | Path | Headers | Query |
|---|---|---|---|---|
| getReviewEligibility | GET | `user-review/fam/gizmo/v5/review` | `x-fp-identifier-profileid`, `x-fp-identifier-target-serviceid` | — |
| updateReviewAction | PATCH | `user-review/fam/gizmo/v5/review` | `x-fp-identifier-profileid`, `x-fp-identifier-target-serviceid` | — |
| submitReview | POST | `user-review/fam/gizmo/v5/review` | `x-fp-identifier-profileid`, `x-fp-identifier-target-serviceid` | — |

## calendarsync (11)

| Method | HTTP | Path | Headers | Query |
|---|---|---|---|---|
| deleteCalendarSelection | DELETE | `/geocal/frisco/v1/schools/selection` | `(dynamic @HeaderMap Map<String,Object>)` | `schoolId`, `subscriptionType` |
| getCalendarEvents | GET | `/geocal/frisco/v1/school/calendar/events` | `(dynamic @HeaderMap Map<String,Object>)` | `filter`, `timezone` |
| getSchoolSuggestion | GET | `/geocal/frisco/v1/school/calendar/suggestions` | `(dynamic @HeaderMap Map<String,Object>)` | — |
| getSchoolsForZipCode | GET | `/geocal/frisco/v1/schools` | `(dynamic @HeaderMap Map<String,Object>)` | `zipcode` |
| getCalendarSelection | GET | `/geocal/frisco/v1/schools/selection` | `(dynamic @HeaderMap Map<String,Object>)` | — |
| getSchoolSelection | GET | `/geocal/frisco/v1/schools/selection` | `(dynamic @HeaderMap Map<String,Object>)` | — |
| updateCalendarSyncPreferences | PATCH | `/geocal/frisco/v1/schools/selection` | `(dynamic @HeaderMap Map<String,Object>)` | — |
| getGoogleCalendarTokens | POST | `/auth/frisco/frisco-iam-device-auth/v7/user/auth/token` | `(dynamic @HeaderMap Map<String,Object>)` | — |
| createCalendarSelection | POST | `/geocal/frisco/v1/schools/selection` | `(dynamic @HeaderMap Map<String,Object>)` | — |
| completeResyncOperation | POST | `/geocal/frisco/v1/schools/selection/calendarsynced` | `(dynamic @HeaderMap Map<String,Object>)` | `schoolId` |
| updateCalendarSelection | PUT | `/geocal/frisco/v1/schools/selection` | `(dynamic @HeaderMap Map<String,Object>)` | — |

## callsAndTexts (10)

| Method | HTTP | Path | Headers | Query |
|---|---|---|---|---|
| deleteContactFromTheList | DELETE | `/vsf/callandtext/v5/managecontacts` | `x-fp-identifier-target-serviceid` | `profileId`, `contactType`, `contactInfo`, `deviceId` |
| getCallAndTextActivityListV7 | GET | `/callandtext/frisco/callandtext/v7/activity` | `x-fp-identifier-target-serviceid` | `summaryOnly`, `startDate`, `endDate`, `activityType`, `timezone`, `betaProviders` |
| getCallAndTextSpecificContactActivityListV7 | GET | `/callandtext/frisco/callandtext/v7/activity/contact` | `x-fp-identifier-target-serviceid` | `otherPartyMdn`, `startDate`, `endDate`, `activityType`, `timezone`, `betaProviders` |
| getCallAndTextProfileSummaryListV7 | GET | `/callandtext/frisco/callandtext/v7/activity/summary` | `x-fp-identifier-target-serviceid` | `startDate`, `endDate`, `summaryOnly`, `activityType`, `timezone`, `betaProviders` |
| getTopContactListByActivityV7 | GET | `/callandtext/frisco/callandtext/v7/activity/top` | `x-fp-identifier-target-serviceid` | `profileId`, `deviceId`, `startDate`, `endDate`, `activityType`, `timezone`, `betaProviders` |
| getAllTrustBlockWatchContactList | GET | `/vsf/callandtext/v5/managecontacts` | `x-fp-identifier-target-serviceid` | `profileId`, `deviceId`, `contactType` |
| getSchedulesRequest | GET | `parental-control/frisco/v6/schedules` | `x-fp-identifier-target-serviceid` | — |
| addNamesToAddressBook | POST | `/frisco/callandtext/v5/addressbook` | `x-fp-identifier-target-serviceid`, `x-fp-identifier-deviceid` | — |
| addContactToTheList | POST | `/vsf/callandtext/v5/managecontacts` | `x-fp-identifier-target-serviceid` | — |
| addCustomNameToAddressBook | PUT | `/frisco/callandtext/v5/addressbook` | `x-fp-identifier-target-serviceid`, `x-fp-identifier-deviceid` | — |

## cancel-subscription (1)

| Method | HTTP | Path | Headers | Query |
|---|---|---|---|---|
| cancelSubscription | DELETE | `/account/fam/userprofile-management/v5/accounts/subscription` | `x-fp-identifier-target-serviceid` | — |

## contactmanagement (16)

| Method | HTTP | Path | Headers | Query |
|---|---|---|---|---|
| removeContact | DELETE | `/vsf/callandtext/v5/gizmo/managecontacts` | `x-fp-identifier-target-serviceid` | `contactType`, `contactInfo` |
| bulkDeleteContacts | DELETE | `callandtext/vsf/callandtext/v5/gizmo/managecontacts/bulkdelete` | `x-fp-identifier-target-serviceid` | — |
| getInAppBanner | GET | `/callandtext/frisco/mappcontent/v5/inappbanner` | `x-fp-identifier-target-serviceid` | `pageId` |
| getTrustedContacts | GET | `/callandtext/vsf/callandtext/v1/feature/settings` | `x-fp-identifier-target-serviceid` | `settingId` |
| getFamilyMembersAndBuddies | GET | `/vsf/callandtext/v5/gizmo/managecontacts` | `x-fp-identifier-target-serviceid` | `contactType`, `filterAvailableMdns` |
| getGizmoContacts | GET | `/vsf/callandtext/v5/gizmo/managecontacts` | `x-fp-identifier-target-serviceid` | `contactType`, `filterPermissions` |
| getGizmoContactImage | GET | `/vsf/callandtext/v5/gizmo/managecontacts/image` | `x-fp-identifier-target-serviceid`, `If-None-Match` | `contactType`, `contactInfo`, `contactId` |
| acceptDeclineFamilyInvite | PATCH | `/fam/userprofile-management/v5/userprofiles/services/invites` | `x-fp-identifier-target-serviceid` | — |
| validateGizmoMdn | POST | `/account/fam/userprofile-management/v5/contact/phonenumber/validate` | `x-fp-identifier-target-serviceid` | — |
| updatePermissions | POST | `/account/fam/userprofile-management/v6/accounts/userprofiles/gizmo/featurepermission` | `x-fp-identifier-target-serviceid` | — |
| updateTrustedContacts | POST | `/callandtext/vsf/callandtext/v1/feature/settings` | `x-fp-identifier-target-serviceid` | — |
| sendInvite | POST | `/fam/userprofile-management/v5/accounts/userprofiles` | `x-fp-identifier-target-serviceid` | — |
| addCallingContact | POST | `/vsf/callandtext/v5/gizmo/managecontacts` | `x-fp-identifier-target-serviceid` | — |
| acceptDeclineBuddyRequest | PUT | `/callandtext/vsf/callandtext/v5/gizmo/managecontacts/acceptinvite` | `x-fp-identifier-target-serviceid` | — |
| updateContact | PUT | `/vsf/callandtext/v5/gizmo/managecontacts` | `x-fp-identifier-target-serviceid` | — |
| bulkUpdateContacts | PUT | `callandtext/vsf/callandtext/v5/gizmo/managecontacts/bulkupdate` | `x-fp-identifier-target-serviceid` | — |

## contentfilter (13)

| Method | HTTP | Path | Headers | Query |
|---|---|---|---|---|
| deleteAppLimit | DELETE | `/parental-control/frisco/v7/app-limits` | `x-fp-identifier-target-serviceid` | `appLimitsId` |
| getFilterContent | GET | `/parental-control/frisco/v7/filterContent` | `app-name`, `x-fp-identifier-target-serviceid` | `include-description` |
| getParentalControls | GET | `/parental-control/frisco/v7/parentalControls` | `x-fp-identifier-target-serviceid` | `profileId` |
| getSubCategoriesDetail | GET | `/parental-control/frisco/v7/subcategory` | `app-name`, `x-fp-identifier-target-serviceid` | `subcategory-id` |
| getCategories | GET | `/parental-control/vsf/v7/categories` | `app-name`, `x-fp-identifier-target-serviceid` | `group-name`, `include-description`, `include-safesearch`, `categorySupported`, `resolution`, `sz`, `r` |
| getObjectionableSettings | GET | `/vsf/commsplatform/v5/report-settings?reportCategory=individual` | `x-fp-identifier-target-serviceid` | `reportCategory` |
| getAgeGroupMetaData | GET | `/vsf/parental-control/v5/age-groups` | `x-fp-identifier-target-serviceid` | `profileId` |
| createAppLimit | POST | `/parental-control/frisco/v7/app-limits` | `x-fp-identifier-target-serviceid` | — |
| setCFCategories | POST | `/parental-control/frisco/v8/categories` | `x-fp-identifier-target-serviceid` | — |
| createGroupPolicy | POST | `/parental-control/frisco/v8/create-group-policies` | `x-fp-identifier-target-serviceid` | `categorySupported` |
| updateSubcategory | POST | `/parental-control/frisco/v8/subcategory` | `x-fp-identifier-target-serviceid` | — |
| postObjectionableSettings | POST | `/vsf/commsplatform/v5/report-settings?reportCategory=individual` | `x-fp-identifier-target-serviceid` | `reportCategory` |
| updateAppLimit | PUT | `/parental-control/frisco/v7/app-limits` | `x-fp-identifier-target-serviceid` | `appLimitsId` |

## cross-sell-card (7)

| Method | HTTP | Path | Headers | Query |
|---|---|---|---|---|
| deleteCrossSellRecommendation | DELETE | `/account/fam/userprofile-management/v5/userprofiles/recommendations` | `x-fp-identifier-target-serviceid` | `contentId`, `id` |
| deleteUpSellRecommendation | DELETE | `/account/fam/userprofile-management/v5/userprofiles/recommendations/promotions` | `x-fp-identifier-target-serviceid` | `contentId`, `id`, `screenTag` |
| getTrialEligibility | GET | `/account/fam/userprofile-management/v5/accounts/trials/eligibility` | `x-fp-identifier-target-serviceid` | — |
| getEnrollFreeTrialData | GET | `/account/fam/userprofile-management/v5/accounts/trials/subscriptions` | `x-fp-identifier-target-serviceid` | — |
| getCrossSellRecommendations | GET | `/account/fam/userprofile-management/v5/userprofiles/recommendations` | `x-fp-identifier-target-serviceid` | — |
| getUpSellRecommendations | GET | `/account/fam/userprofile-management/v5/userprofiles/recommendations/promotions` | `x-fp-identifier-target-serviceid` | — |
| getPetTrackerPurchaseLink | GET | `/tracker/fam/v1/purchaseLink` | `x-fp-identifier-target-serviceid` | — |

## device-settings-appmanagement (2)

| Method | HTTP | Path | Headers | Query |
|---|---|---|---|---|
| getAppStatus | GET | `/account/fam/device-management/v5/device/app/management` | `x-fp-identifier-target-serviceid` | — |
| updateAppStatus | POST | `/account/fam/device-management/v5/device/app/management` | `x-fp-identifier-target-serviceid` | — |

## deviceSettings (5)

| Method | HTTP | Path | Headers | Query |
|---|---|---|---|---|
| getDeviceLogDownloadUrl | GET | `/comms/fam/v1/device/logs/download` | `x-trace-transaction-id`, `x-fp-identifier-target-serviceid` | `id` |
| getDeviceLogs | GET | `/comms/fam/v1/device/logs/list` | `x-trace-transaction-id`, `x-fp-identifier-target-serviceid` | — |
| getDeviceSettings | GET | `/fam/comms/v1/device/settings` | `x-trace-transaction-id`, `x-fp-identifier-target-serviceid` | `settingsType`, `timezone` |
| triggerLogUpload | POST | `/comms/fam/v1/device/logs/retrieve` | `x-trace-transaction-id`, `x-fp-identifier-target-serviceid` | — |
| postDeviceSettings | POST | `/fam/comms/v1/device/settings` | `x-trace-transaction-id`, `x-fp-identifier-target-serviceid` | — |

## driving-insights (9)

| Method | HTTP | Path | Headers | Query |
|---|---|---|---|---|
| getSettings | GET | `/frisco/driving-insights/v5/settings` | `x-fp-identifier-target-serviceid` | — |
| getAuthCode | GET | `/vsf/driving-insights/v5/auth-code/` | `x-fp-identifier-target-serviceid` | — |
| getTripDetail | GET | `/vsf/driving-insights/v5/trip-details/` | `x-fp-identifier-target-serviceid` | `driveId` |
| getTripSummary | GET | `/vsf/driving-insights/v5/trip-summary/` | `x-fp-identifier-target-serviceid` | `timezone` |
| getTrips | GET | `/vsf/driving-insights/v5/trips/` | `x-fp-identifier-target-serviceid` | `timeZone` |
| patchTransportationMode | PATCH | `/vsf/driving-insights/v5/transportationMode/` | `x-fp-identifier-target-serviceid` | `driveId` |
| postSettings | POST | `/frisco/driving-insights/v5/settings` | `x-fp-identifier-target-serviceid` | — |
| putSettings | PUT | `/frisco/driving-insights/v5/settings` | `x-fp-identifier-target-serviceid` | — |
| putSpeedAlertLimit | PUT | `/frisco/driving-insights/v5/settings` | `x-fp-identifier-target-serviceid` | — |

## driving-insights-crash (5)

| Method | HTTP | Path | Headers | Query |
|---|---|---|---|---|
| getCrashNotifications | GET | `/frisco/driving-insights/v5/crash-notifications` | `crash-date-time`, `x-fp-identifier-target-serviceid` | — |
| getCrashNotifications | GET | `/frisco/driving-insights/v5/crash-notifications` | `x-fp-identifier-target-serviceid` | — |
| getCrashDetail | GET | `safety/vsf/driving-insights/v5/crashes/{crashId}` | `x-fp-identifier-target-serviceid` | — |
| patchCrashNotification | PATCH | `safety/vsf/driving-insights/v6/crash-notifications` | `x-fp-identifier-target-serviceid` | — |
| updateCrashNotifications | POST | `/frisco/driving-insights/v5/crash-notifications` | `x-fp-identifier-target-serviceid` | — |

## emergencycontacts (5)

| Method | HTTP | Path | Headers | Query |
|---|---|---|---|---|
| deleteEmergencyContact | DELETE | `/frisco/userprofile-management/v5/contacts/{contact-id}` | `x-fp-identifier-target-serviceid` | — |
| getAllAvailableEmergencyContactsBasedOnAccountId | GET | `/fam/userprofile-management/v5/accounts/contacts` | `x-fp-identifier-target-serviceid` | — |
| getAllAvailableEmergencyContacts | GET | `/fam/userprofile-management/v5/userprofiles/contacts` | `x-fp-identifier-target-serviceid` | — |
| addEmergencyContactsToProfile | POST | `/fam/userprofile-management/v5/userprofiles/contacts` | `x-fp-identifier-target-serviceid` | — |
| updateEmergencyContacts | PUT | `account/fam/userprofile-management/v5/userprofiles/contacts/replace` | `x-fp-identifier-target-serviceid` | — |

## family-line (19)

| Method | HTTP | Path | Headers | Query |
|---|---|---|---|---|
| deProvisionFamilyLine | DELETE | `/account/fam/userprofile-management/v5/accounts/subscriptions/feature/FAMILYLINE` | `@HeaderMap (dynamic)` | — |
| removeUserFromFamilyLine | DELETE | `/callandtext/frisco/familyline/v1/fl/line` | `@HeaderMap (dynamic)` | — |
| getAddress | GET | `/callandtext/frisco/familyline/v1/fl/address` | `@HeaderMap (dynamic)` | — |
| getFamilyLines | GET | `/callandtext/frisco/familyline/v1/fl/line` | `@HeaderMap (dynamic)` | — |
| getEligibleLines | GET | `/callandtext/frisco/familyline/v1/fl/line/eligible` | `@HeaderMap (dynamic)` | — |
| logoutFamilyLine | GET | `/callandtext/frisco/familyline/v1/fl/line/logout` | `@HeaderMap (dynamic)` | — |
| getProvisioningStatus | GET | `/callandtext/frisco/familyline/v1/fl/status` | `@HeaderMap (dynamic)` | — |
| provisionUser | POST | `/account/fam/userprofile-management/v5/accounts/subscriptions/feature/FAMILYLINE` | `@HeaderMap (dynamic)` | — |
| validateAddress | POST | `/account/fam/userprofile-management/v5/userprofiles/address/validate` | `@HeaderMap (dynamic)` | — |
| saveAddress | POST | `/callandtext/frisco/familyline/v1/fl/address` | `@HeaderMap (dynamic)` | — |
| sendCallLog | POST | `/callandtext/frisco/familyline/v1/fl/call/logs` | `@HeaderMap (dynamic)` | — |
| acceptFamilyLineInvite | POST | `/callandtext/frisco/familyline/v1/fl/line` | `@HeaderMap (dynamic; x-fp-identifier-* supplied at runtime)` | — |
| sendFamilyLineInvite | POST | `/callandtext/frisco/familyline/v1/fl/line/invite` | `@HeaderMap (dynamic)` | — |
| updateAppUuid | POST | `/callandtext/frisco/familyline/v1/fl/line/updateAppUuid` | `@HeaderMap (dynamic)` | — |
| updateFamilyLineSettings | POST | `/callandtext/frisco/familyline/v1/fl/settings` | `@HeaderMap (dynamic)` | — |
| swapDeviceProvisioning | POST | `/callandtext/frisco/familyline/v1/fl/swap-device` | `@HeaderMap (dynamic)` | — |
| termsAndConditionsAccepted | POST | `/callandtext/frisco/familyline/v1/fl/tc` | `@HeaderMap (dynamic)` | — |
| getSpcToken | POST | `/callandtext/frisco/familyline/v1/fl/token` | `@HeaderMap (dynamic)` | — |
| traceSdkResponse | POST | `/callandtext/frisco/familyline/v1/fl/trace` | `@HeaderMap (dynamic)` | — |

## flightdetection (3)

| Method | HTTP | Path | Headers | Query |
|---|---|---|---|---|
| getFlightDetectionStatus | GET | `location/vsf/location/v5/flight/settings` | `x-fp-identifier-target-serviceid` | — |
| updateFlightDetectionStatus | POST | `location/vsf/location/v5/locationResponse` | `x-fp-identifier-target-serviceid` | — |
| updateFlightDetectionSetting | PUT | `location/vsf/location/v5/flight/settings` | `x-fp-identifier-target-serviceid` | — |

## gizmo-activation (1)

| Method | HTTP | Path | Headers | Query |
|---|---|---|---|---|
| validateGizmoActivation | POST | `/account/fam/userprofile-management/v5/gizmo/activation/validate` | `x-fp-identifier-target-serviceid` | — |

## identity-audit (2)

| Method | HTTP | Path | Headers | Query |
|---|---|---|---|---|
| postBiometricAudit | POST | `/frisco/frisco-iam-device-auth/v5/user/auth/audit` | — | — |
| auditLogin | POST | `auth/frisco/frisco-iam-device-auth/v5/user/login/audit` | — | — |

## identity-login (4)

| Method | HTTP | Path | Headers | Query |
|---|---|---|---|---|
| loginActivity | POST | `/auth/frisco/frisco-iam-device-auth/v7/user/auth/l1/login/activity` | — | — |
| getChildDeviceAccessToken | POST | `/frisco/frisco-iam-device-auth/v5/deviceauth/auth/token` | — | — |
| logOut | POST | `/frisco/frisco-iam-device-auth/v5/user/auth/logout` | — | — |
| getAppLoginTokens | POST | `{authPath}` | — | — |

## identity-refresh-tokens (2)

| Method | HTTP | Path | Headers | Query |
|---|---|---|---|---|
| childRefreshToken | POST | `/auth/frisco/frisco-iam-device-auth/v6/deviceauth/refreshtoken` | — | — |
| refreshToken | POST | `/auth/frisco/frisco-iam-device-auth/v7/user/auth/token` | — | — |

## identity-verify-device (5)

| Method | HTTP | Path | Headers | Query |
|---|---|---|---|---|
| sendAuthOtp | POST | `/auth/frisco/frisco-iam-device-auth/v7/user/auth/otp` | — | — |
| validateAuthOtp | POST | `/auth/frisco/frisco-iam-device-auth/v7/user/auth/otp/validate` | — | — |
| sendOtp | POST | `/auth/frisco/frisco-iam-user-auth/v6/user/otp` | — | — |
| validateOtp | POST | `/auth/frisco/frisco-iam-user-auth/v6/user/otp/validate` | — | — |
| verifyMdn | POST | `/fam/userprofile-management/v5/phonenumber/validate` | — | — |

## installedapps (1)

| Method | HTTP | Path | Headers | Query |
|---|---|---|---|---|
| sendInstalledApps | POST | `/parental-control/frisco/v5/installed-apps` | `x-fp-identifier-target-serviceid` | — |

## invite (4)

| Method | HTTP | Path | Headers | Query |
|---|---|---|---|---|
| getAccountLines | GET | `/account/fam/userprofile-management/v6/accounts/lines` | `x-fp-identifier-target-serviceid` | — |
| createWifiDevice | POST | `/account/fam/userprofile-management/v6/accounts/userprofiles` | `x-fp-identifier-target-serviceid` | — |
| sendInvite | POST | `/account/fam/userprofile-management/v6/accounts/userprofiles` | `x-fp-identifier-target-serviceid` | — |
| setRelationships | PUT | `/callandtext/vsf/callandtext/v5/gizmo/managecontacts/bulkupdate` | `x-fp-identifier-target-serviceid` | — |

## legal-privacy-data (2)

| Method | HTTP | Path | Headers | Query |
|---|---|---|---|---|
| getDownloadLink | GET | `/account/frisco/customer/v5/userprofiles/privacydata/download` | `x-fp-identifier-target-serviceid` | — |
| requestPrivacyDataDownload | POST | `/account/frisco/customer/v5/userprofiles/privacydata/download` | `x-fp-identifier-target-serviceid` | — |

## location (19)

| Method | HTTP | Path | Headers | Query |
|---|---|---|---|---|
| deleteGeofenceSettings | DELETE | `/vsf/location/v6/geofence/settings` | `x-fp-identifier-target-serviceid` | `eventId`, `eventType` |
| getDashboardDetails | GET | `/vsf/location/v5/dashboard` | `x-fp-identifier-target-serviceid` | `locationEnabled`, `locPermission`, `onlyLastKnownLoc`, `source`, `onDemand` |
| getPickMeUpStatus | GET | `/vsf/location/v5/events/active` | `x-fp-identifier-target-serviceid` | `profileId`, `eventType`, `eventStatus`, `profileRole` |
| fetchHistory | GET | `/vsf/location/v5/history` | `x-fp-identifier-target-serviceid` | `profileId`, `startTime` |
| getHistoryStatus | GET | `/vsf/location/v5/history/available` | `x-fp-identifier-target-serviceid` | `profileId` |
| getAvailableParentForPickMeUp | GET | `vsf/location/v5/events/active?eventType=pickMeUp&profileRole=CHILD&operation=getAvailableParents` | `x-fp-identifier-target-serviceid` | `eventType=pickMeUp`, `profileRole=CHILD`, `operation=getAvailableParents` |
| getLocationSharingSettings | GET | `vsf/location/v5/settings` | `x-fp-identifier-target-serviceid` | `eventType`, `profileId` |
| getLocationSharingConfigEvent | GET | `vsf/location/v6/settings` | `x-fp-identifier-target-serviceid` | `eventType`, `profileId` |
| getWithWhomIamSharingLocation | GET | `vsf/location/v6/settings` | `x-fp-identifier-target-serviceid` | `eventType`, `profileId` |
| checkIn | POST | `/vsf/location/v5/checkin` | `x-fp-identifier-target-serviceid` | — |
| postGeofenceViolationEvent | POST | `/vsf/location/v5/geofence/event` | `x-fp-identifier-target-serviceid` | — |
| manageLiveLocationRequest | POST | `/vsf/location/v5/live` | `x-fp-identifier-target-serviceid` | — |
| postLocationTamper | POST | `/vsf/location/v5/tamper` | `x-fp-identifier-target-serviceid` | — |
| pickMeUp | POST | `vsf/location/v5/pickmeup` | `x-fp-identifier-target-serviceid` | — |
| checkInSeen | PUT | `/vsf/location/v5/checkin` | `x-fp-identifier-target-serviceid` | — |
| putPickMeUp | PUT | `/vsf/location/v5/pickmeup` | `x-fp-identifier-target-serviceid` | — |
| updateLocationSharingSetting | PUT | `/vsf/location/v5/settings` | `x-fp-identifier-target-serviceid` | — |
| updateLocationSharingSettingConfig | PUT | `/vsf/location/v6/settings` | `x-fp-identifier-target-serviceid` | `eventType`, `profileId` |
| sendGeoFenceConfirmation | PUT | `vsf/location/v6/geofence/settings?operation=configGeoDevice` | `x-fp-identifier-target-serviceid` | `operation=configGeoDevice` |

## location-geofence (4)

| Method | HTTP | Path | Headers | Query |
|---|---|---|---|---|
| getGeofenceSettings | GET | `/vsf/location/v6/geofence/settings` | `x-fp-identifier-target-serviceid` | `eventType`, `withGeofence` |
| getSavedLocations | GET | `/vsf/location/v6/geofence/settings` | `x-fp-identifier-target-serviceid` | `eventType`, `withGeofence`, `strategy` |
| createDeviceGeofenceSettings | POST | `/vsf/location/v6/geofence/settings` | `x-fp-identifier-target-serviceid` | — |
| updateDeviceGeofenceSettings | PUT | `/vsf/location/v6/geofence/settings` | `x-fp-identifier-target-serviceid` | — |

## location-schedulealert (4)

| Method | HTTP | Path | Headers | Query |
|---|---|---|---|---|
| deleteScheduledAlert | DELETE | `/vsf/location/v5/scheduled/alert/settings` | `x-fp-identifier-target-serviceid` | — |
| getScheduledAlerts | GET | `/vsf/location/v5/scheduled/alert/settings` | `x-fp-identifier-target-serviceid` | `eventType`, `profileRole`, `profileId` |
| postScheduleAlert | POST | `/vsf/location/v5/scheduled/alert/settings` | `x-fp-identifier-target-serviceid` | — |
| updateScheduledAlert | PUT | `/vsf/location/v5/scheduled/alert/settings` | `x-fp-identifier-target-serviceid` | — |

## managecontact (4)

| Method | HTTP | Path | Headers | Query |
|---|---|---|---|---|
| deleteContactFromTheList | DELETE | `/vsf/callandtext/v5/managecontacts` | `x-fp-identifier-target-serviceid` | `profileId`, `contactType`, `contactInfo`, `deviceId` |
| getAllContactsRequest | GET | `/vsf/callandtext/v5/managecontacts` | `x-fp-identifier-target-serviceid` | `profileId`, `deviceId`, `contactType` |
| addContactToTheList | POST | `/vsf/callandtext/v5/managecontacts` | `x-fp-identifier-target-serviceid` | — |
| putPrivateRestrictedCall | PUT | `/vsf/callandtext/v5/managecontacts` | `x-fp-identifier-target-serviceid` | — |

## medicalid (2)

| Method | HTTP | Path | Headers | Query |
|---|---|---|---|---|
| postMedicalId | POST | `/comms/fam/v1/device/medical-Id` | `x-fp-identifier-target-serviceid` | — |
| putMedicalId | PUT | `/comms/fam/v1/device/medical-Id` | `x-fp-identifier-target-serviceid` | — |

## messaging (19)

| Method | HTTP | Path | Headers | Query |
|---|---|---|---|---|
| deleteGroupChat | DELETE | `account/fam/group-management/v5/groups/{group-id}` | `x-fp-identifier-target-serviceid` | — |
| exitGroup | DELETE | `account/fam/group-management/v5/groups/{group-id}/exit` | `x-fp-identifier-target-serviceid` | — |
| deleteGroupMember | DELETE | `account/fam/group-management/v5/groups/{group-id}/members/{member-id}` | `x-fp-identifier-target-serviceid` | — |
| clearAllGroupChatMessages | DELETE | `callandtext/vsf/callandtext/v5/groupchat/messages` | `x-fp-identifier-target-serviceid` | `chatGroupId` |
| deleteMessages | DELETE | `comms/fam/v1/messages` | `x-fp-identifier-target-serviceid` | — |
| getMessageList | GET | `/comms/fam/v1/messageList` | `x-fp-identifier-target-serviceid` | `messageId`, `limit`, `direction` |
| getMediaMessage | GET | `/comms/fam/v1/messageMedia` | `x-fp-identifier-target-serviceid` | `messageId`, `presignedUrl` |
| getAllGroups | GET | `account/fam/group-management/v5/groups` | `x-fp-identifier-target-serviceid` | — |
| getAllEligibleMembers | GET | `account/fam/group-management/v5/groups/members/eligible` | `x-fp-identifier-target-serviceid` | — |
| getRemainingMembersToAddGroup | GET | `account/fam/group-management/v5/groups/{group-id}/members/eligible` | `x-fp-identifier-target-serviceid` | — |
| getGroupMediaMessage | GET | `callandtext/vsf/callandtext/v5/groupchat/message` | `x-fp-identifier-target-serviceid` | `chatGroupId`, `messageId`, `presignedUrl` |
| updateGroupChat | PATCH | `account/fam/group-management/v5/groups/{group-id}` | `x-fp-identifier-target-serviceid` | — |
| postMessageMedia | POST | `/comms/fam/v1/messageMedia` | `x-transaction-id`, `x-trace-transaction-id`, `x-fp-identifier-target-serviceid` | — |
| createNewGroup | POST | `account/fam/group-management/v5/groups` | `x-fp-identifier-target-serviceid` | — |
| sendGroupMediaMessage | POST | `callandtext/vsf/callandtext/v5/groupchat/message` | `x-fp-identifier-target-serviceid` | `chatGroupId` |
| sendGroupTextMessage | POST | `callandtext/vsf/callandtext/v5/groupchat/message` | `x-fp-identifier-target-serviceid` | `chatGroupId` |
| getAllGroupMessage | POST | `callandtext/vsf/callandtext/v5/groupchat/messages` | `x-fp-identifier-target-serviceid` | — |
| messageRead | POST | `callandtext/vsf/callandtext/v5/groupchat/messages/read` | `x-fp-identifier-target-serviceid` | `chatGroupId` |
| addMembers | PUT | `account/fam/group-management/v5/groups/{group-id}/members` | `x-fp-identifier-target-serviceid` | — |

## most-used-apps (1)

| Method | HTTP | Path | Headers | Query |
|---|---|---|---|---|
| getTopApps | GET | `/parental-control/frisco/v5/web-app-activity/topApps` | `x-fp-identifier-target-serviceid` | `date`, `count`, `timezone` |

## notifications (9)

| Method | HTTP | Path | Headers | Query |
|---|---|---|---|---|
| getFilters | GET | `/comms/frisco/v5/notifications/filters` | `x-fp-identifier-target-serviceid` | `resolution`, `sz` |
| getNotifications | GET | `/comms/frisco/v6/notifications` | `x-fp-identifier-target-serviceid` | `page`, `per-page`, `filters`, `productType`, `notificationType`, `exclude` |
| getReportSettings | GET | `/comms/vsf/v8/report-settings` | `x-fp-identifier-target-serviceid` | — |
| getNotificationFeed | GET | `/frisco/commsplatform/v5/notifications` | `x-fp-identifier-target-serviceid` | `productType`, `notificationType`, `exclude` |
| getObjectionableWeb | GET | `/frisco/parental-control/v5/noteworthy-details/{eventId}` | `x-fp-identifier-target-serviceid` | `notification-type` |
| getReportSettings | GET | `/vsf/commsplatform/v5/report-settings?reportCategory=individual` | `x-fp-identifier-target-serviceid` | `reportCategory` |
| getNotificationCount | GET | `comms/frisco/v6/notifications/unreadCount` | `x-fp-identifier-target-serviceid` | `productType`, `notificationType` |
| markAllRead | PATCH | `/comms/fam/v6/notifications/updateAll` | `x-fp-identifier-target-serviceid` | `productType`, `notificationType` |
| updateReportSettings | POST | `/comms/vsf/v7/report-settings` | `x-fp-identifier-target-serviceid` | `reportCategory` |

## onboarding (1)

| Method | HTTP | Path | Headers | Query |
|---|---|---|---|---|
| getProfileAvatars | GET | `/parental-control/frisco/v5/profile-avatars` | `x-fp-identifier-target-serviceid` | `resolution` |

## pairing-device-gizmo (48)

| Method | HTTP | Path | Headers | Query |
|---|---|---|---|---|
| deleteMediaBackupEntries | DELETE | `/comms/fam/v1/device/backup/media` | `x-fp-identifier-target-serviceid` | — |
| unlinkGizmoAccount | DELETE | `/fam/userprofile-management/v5/userprofiles/gizmo/unlink` | `x-trace-transaction-id` | — |
| gizmoImportDevicesNotSignedInUser | GET | `/account/fam/userprofile-management/v1/gizmo/import/devices` | `x-fp-identifier-target-serviceid` | `canLink` |
| gizmoImportEligibilityNotSignedInUser | GET | `/account/fam/userprofile-management/v1/gizmo/import/eligibility` | `x-fp-identifier-target-serviceid` | — |
| gizmoImportDevices | GET | `/account/fam/userprofile-management/v5/gizmo/import/devices` | `x-transaction-id`, `x-fp-identifier-target-serviceid` | `canLink` |
| gizmoImportEligibility | GET | `/account/fam/userprofile-management/v5/gizmo/import/eligibility` | `x-transaction-id`, `x-fp-identifier-target-serviceid` | — |
| gizmoDevicesCheckEligibility | GET | `/account/fam/userprofile-management/v5/user-profiles/gizmo-devices` | `x-transaction-id`, `x-identifier-mdn`, `x-fp-identifier-target-serviceid` | `eligibility-check` |
| gizmoDevicesGetLists | GET | `/account/fam/userprofile-management/v5/user-profiles/gizmo-devices` | `x-transaction-id`, `x-identifier-mdn`, `x-fp-identifier-target-serviceid` | `eligibility-check` |
| getInteractionData | GET | `/comms/fam/v1/device/ai/logs` | `x-fp-identifier-target-serviceid` | `logId`, `limit`, `direction` |
| getMediaBackupList | GET | `/comms/fam/v1/device/backup/mediaList` | `x-fp-identifier-target-serviceid` | `childId`, `mediaId`, `direction`, `limit` |
| getDeviceLogDownloadUrl | GET | `/comms/fam/v1/device/logs/download` | `x-trace-transaction-id`, `x-fp-identifier-target-serviceid` | `id` |
| getDeviceLogs | GET | `/comms/fam/v1/device/logs/list` | `x-trace-transaction-id`, `x-fp-identifier-target-serviceid` | — |
| getMediaList | GET | `/comms/fam/v1/device/mediaList` | `x-fp-identifier-target-serviceid`, `gizmo-device-model` | — |
| getDeviceStatus | GET | `/comms/fam/v1/device/status` | `x-fp-identifier-target-serviceid` | `onDemand` |
| getMediaBackupStorageStatus | GET | `/comms/fam/v1/device/status` | `x-fp-identifier-target-serviceid` | `statusType` |
| getGizmoDevices | GET | `/fam/comms/v1/device` | `x-fp-identifier-target-serviceid` | — |
| getDeviceSettings | GET | `/fam/comms/v1/device/settings` | `x-trace-transaction-id`, `x-fp-identifier-target-serviceid` | `settingsType`, `timezone` |
| getMdnLookupResponse | GET | `/frisco/comms/v1/gizmo-mdn-lookup` | `x-fp-identifier-target-serviceid`, `x-mdn` | — |
| getNotificationFeed | GET | `/frisco/commsplatform/v5/notifications` | `x-fp-identifier-target-serviceid` | `productType`, `notificationType`, `exclude` |
| getOtpStatus | GET | `/frisco/frisco-iam-user-auth/v5/wifi/otp` | `x-fp-identifier-target-serviceid` | — |
| getObjectionableWeb | GET | `/frisco/parental-control/v5/noteworthy-details/{eventId}` | `x-fp-identifier-target-serviceid` | `notification-type` |
| getConsent | GET | `/frisco/parental-control/v5/profiles/devices/consent` | `x-fp-identifier-target-serviceid` | — |
| getDefaultDOHLocation | GET | `/frisco/parental-control/v5/provider/account/location` | `x-fp-identifier-target-serviceid` | — |
| getWebAppVisibility | GET | `/frisco/parental-control/v5/web-app-activity/visibility` | `x-fp-identifier-target-serviceid` | `vpn-retry-type` |
| getReportSettings | GET | `/vsf/commsplatform/v5/report-settings?reportCategory=individual` | `x-fp-identifier-target-serviceid` | `reportCategory` |
| getDeviceStatus | GET | `/vsf/device/v5/deviceStatus` | `x-fp-identifier-target-serviceid` | `deviceId` |
| getDeviceShadowDetails | GET | `vsf/tamper/v6/view/getDeviceShadowDetails` | `x-fp-identifier-target-serviceid` | `deviceID`, `featureIds`, `retrieveLevel` |
| reSendInvite | PATCH | `/fam/userprofile-management/v5/userprofiles/services/invites` | `x-fp-identifier-target-serviceid` | — |
| gizmoImportInitiateNotSignedInUser | POST | `/account/fam/userprofile-management/v1/gizmo/import/initiate` | `x-fp-identifier-target-serviceid` | — |
| validateGizmoActivation | POST | `/account/fam/userprofile-management/v5/gizmo/activation/validate` | `x-fp-identifier-target-serviceid` | — |
| gizmoImportInitiate | POST | `/account/fam/userprofile-management/v5/gizmo/import/initiate` | `x-transaction-id`, `x-fp-identifier-target-serviceid` | — |
| validateGizmoMdn | POST | `/account/fam/userprofile-management/v5/gizmo/pairing/validate` | `@HeaderMap (dynamic Map<String,String>)`, `x-fp-identifier-target-serviceid` | — |
| findGizmo | POST | `/comms/fam/v1/device/findMy` | `x-fp-identifier-target-serviceid` | — |
| upgrade | POST | `/comms/fam/v1/device/fota/upgrade` | `x-fp-identifier-target-serviceid` | — |
| checkUpgrade | POST | `/comms/fam/v1/device/fota/upgradeCheck` | `x-fp-identifier-target-serviceid` | — |
| triggerLogUpload | POST | `/comms/fam/v1/device/logs/retrieve` | `x-trace-transaction-id`, `x-fp-identifier-target-serviceid` | — |
| powerOffGizmoDevice | POST | `/comms/fam/v1/device/powerOff` | `x-fp-identifier-target-serviceid` | — |
| postDeviceSettings | POST | `/fam/comms/v1/device/settings` | `x-trace-transaction-id`, `x-fp-identifier-target-serviceid` | — |
| createProfile | POST | `/fam/userprofile-management/v5/accounts/userprofiles` | `x-fp-identifier-target-serviceid` | — |
| createDependentProfile | POST | `/fam/userprofile-management/v5/accounts/userprofiles` | `x-fp-identifier-target-serviceid` | — |
| addDeviceToProfile | POST | `/fam/userprofile-management/v5/accounts/userprofiles/devices` | `x-fp-identifier-target-serviceid` | — |
| addDevice | POST | `/fam/userprofile-management/v5/accounts/userprofiles/devices` | `x-fp-identifier-target-serviceid` | — |
| getIdTokenUsingOtp | POST | `/frisco/frisco-iam-device-auth/v5/wifi/token` | `@HeaderMap (dynamic Map<String,Object>)` | — |
| linkGizmoAccount | POST | `/frisco/frisco-iam-gizmo-auth/v1/gizmo/auth/token` | `x-trace-transaction-id` | — |
| resendOtp | POST | `/frisco/frisco-iam-user-auth/v5/wifi/otp` | `x-fp-identifier-target-serviceid` | — |
| setConsent | POST | `/frisco/parental-control/v5/profiles/consent` | `x-fp-identifier-target-serviceid` | — |
| updateProfileImage | PUT | `/fam/userprofile-management/v5/accounts/userprofiles/images` | `x-fp-identifier-target-serviceid` | — |
| updatePairing | PUT | `/fam/userprofile-management/v5/userprofiles/pairing/devices` | `x-fp-identifier-target-serviceid` | — |

## pauseinternet (3)

| Method | HTTP | Path | Headers | Query |
|---|---|---|---|---|
| unPauseInternet | DELETE | `/frisco/parental-control/v5/device/pause` | `x-fp-identifier-target-serviceid` | — |
| getDevices | GET | `/parental-control/frisco/v6/device/pause` | `x-fp-identifier-target-serviceid` | — |
| pauseInternet | POST | `/frisco/parental-control/v5/device/pause` | `x-fp-identifier-target-serviceid` | — |

## petlivetracker (7)

| Method | HTTP | Path | Headers | Query |
|---|---|---|---|---|
| getAllAvailableEmergencyContacts | GET | `/fam/userprofile-management/v5/userprofiles/contacts` | `x-fp-identifier-target-serviceid` | — |
| getPetLiveTrackerSessionInfo | GET | `safety/fam/pet-tracker/v6/sessions` | `x-fp-identifier-target-serviceid` | — |
| getPetTrackerLocationHistory | GET | `safety/fam/pet-tracker/v6/sessions/{session-id}/location/history` | `x-fp-identifier-target-serviceid` | — |
| getPetTrackerSMSLink | GET | `safety/fam/pet-tracker/v6/sessions/{session-id}/sms/url` | `x-fp-identifier-target-serviceid` | — |
| endRttOrLpm | PATCH | `safety/fam/pet-tracker/v6/sessions` | `x-fp-identifier-target-serviceid` | — |
| escalateRttToLpm | PATCH | `safety/fam/pet-tracker/v6/sessions` | `x-fp-identifier-target-serviceid` | — |
| submitPetLiveTracker | POST | `safety/fam/pet-tracker/v6/sessions` | `x-fp-identifier-target-serviceid` | — |

## pettracker (17)

| Method | HTTP | Path | Headers | Query |
|---|---|---|---|---|
| deleteWifiDetails | DELETE | `/tracker/fam/v1/pet/wifi` | `@HeaderMap` | — |
| getBreeds | GET | `/tracker/fam/v1/breeds` | `@HeaderMap` | `species` |
| getCollarSoftwareInfo | GET | `/tracker/fam/v1/device/status` | `@HeaderMap` | — |
| getTutorialVideoDetails | GET | `/tracker/fam/v1/hw-works` | `@HeaderMap` | — |
| getEncryptionKey | GET | `/tracker/fam/v1/keys` | `@HeaderMap` | — |
| getActivity | GET | `/tracker/fam/v1/pet/activity` | `@HeaderMap`, `duration` | — |
| getActivityV2 | GET | `/tracker/fam/v1/pet/activity` | `@HeaderMap`, `duration` | — |
| getRest | GET | `/tracker/fam/v1/pet/rest` | `@HeaderMap`, `duration` | — |
| getWifiList | GET | `/tracker/fam/v1/pet/wifi` | `@HeaderMap` | — |
| getPurchaseLink | GET | `/tracker/fam/v1/purchaseLink` | `@HeaderMap` | — |
| getStepDistribution | GET | `/tracker/fam/v1/step-distribution` | `@HeaderMap` | `breedId`, `name`, `ageInYears` |
| getReportingToken | POST | `/tracker/fam/v1/pet/mobile-sdk-reporting-token` | `@HeaderMap` | — |
| saveWifiDetails | POST | `/tracker/fam/v1/pet/wifi` | `@HeaderMap` | — |
| getSdkToken | POST | `/tracker/fam/v1/sdk-oauth-token` | `@HeaderMap` | — |
| logFiSdkInteractionEvent | POST | `/tracker/fam/v1/trace/logs` | `@HeaderMap` | — |
| firmwareUpdateStatus | POST | `comms/vsf/tamper/v5/fwUpdate` | `@HeaderMap` | — |
| updateWifiDetails | PUT | `/tracker/fam/v1/pet/wifi` | `@HeaderMap` | — |

## professionalmonitoring (10)

| Method | HTTP | Path | Headers | Query |
|---|---|---|---|---|
| getSubscriberSetupInfo | GET | `fam/prof-monitoring/v5/subscribers` | `x-fp-identifier-target-serviceid` | — |
| getRemoteProfessionalMonitoringAddress | GET | `fam/userprofile-management/v5/userprofiles/address` | `x-fp-identifier-target-serviceid`, `addressType` | — |
| deactivateProfile | PATCH | `/safety/fam/prof-monitoring/v6/subscribers/{subscriber-id}` | `x-fp-identifier-target-serviceid` | — |
| reactivateProfile | PATCH | `/safety/fam/prof-monitoring/v6/subscribers/{subscriber-id}` | `x-fp-identifier-target-serviceid` | — |
| updateFirstAndLastName | PATCH | `fam/userprofile-management/v5/userprofiles` | `x-fp-identifier-target-serviceid` | — |
| validateAddress | POST | `fam/prof-monitoring/v5/addresses/validation` | `x-fp-identifier-target-serviceid` | — |
| createHelp | POST | `fam/prof-monitoring/v5/helps` | `x-fp-identifier-target-serviceid` | — |
| createSubscribers | POST | `fam/prof-monitoring/v5/subscribers` | `x-fp-identifier-target-serviceid` | — |
| updateSubscribers | PUT | `fam/prof-monitoring/v5/subscribers` | `x-fp-identifier-target-serviceid` | — |
| updateAddress | PUT | `fam/userprofile-management/v5/userprofiles/address/{address-id}` | `x-fp-identifier-target-serviceid` | — |

## restrictedUsage (3)

| Method | HTTP | Path | Headers | Query |
|---|---|---|---|---|
| resetLimit | DELETE | `/vsf/callandtext/v5/usagelimits` | `x-fp-identifier-target-serviceid` | — |
| getAllTheLimits | GET | `/vsf/callandtext/v5/usagelimits` | `x-fp-identifier-target-serviceid` | `profileId`, `deviceId`, `limitType` |
| addLimit | PUT | `/vsf/callandtext/v5/usagelimits` | `x-fp-identifier-target-serviceid` | — |

## rsa-roadside-assistance (16)

| Method | HTTP | Path | Headers | Query |
|---|---|---|---|---|
| deleteExistVehicle | DELETE | `fam/rsa/v5/vehicles/{vehicle-id}` | `x-fp-identifier-target-serviceid` | — |
| getRescueInfo | GET | `fam/rsa/v5/rescue` | `x-fp-identifier-target-serviceid` | — |
| getRsaMemberAccess | GET | `fam/rsa/v5/rescue/dependents` | `x-fp-identifier-target-serviceid` | — |
| getRescueHistory | GET | `fam/rsa/v5/rescue/history` | `x-fp-identifier-target-serviceid` | — |
| getCarMakes | GET | `frisco/rsa/v5/makes/{year}` | `x-fp-identifier-target-serviceid` | — |
| updateRsaMemberAccess | PATCH | `fam/rsa/v5/rescue/dependents` | `x-fp-identifier-target-serviceid` | — |
| dismissRsaNotificationBanner | PATCH | `fam/rsa/v5/rescue/setup` | `x-fp-identifier-target-serviceid` | — |
| dismissWhatsNew | PATCH | `safety/fam/rsa/v6/rescue/whatsnew` | `x-fp-identifier-target-serviceid` | — |
| submitRescue | POST | `fam/rsa/v5/rescue` | `x-fp-identifier-target-serviceid` | — |
| setUpRsaIntroBottomSheet | POST | `fam/rsa/v5/rescue/setup` | `x-fp-identifier-target-serviceid` | — |
| getTowLocations | POST | `fam/rsa/v5/rescue/tow/locations` | `x-fp-identifier-target-serviceid` | — |
| validateTowLocation | POST | `fam/rsa/v5/rescue/tow/validate` | `x-fp-identifier-target-serviceid` | — |
| addVehicle | POST | `fam/rsa/v5/vehicles` | `x-fp-identifier-target-serviceid` | — |
| getCarModels | POST | `frisco/rsa/v5/models` | `x-fp-identifier-target-serviceid` | — |
| cancelRequest | PUT | `fam/rsa/v5/rescue` | `x-fp-identifier-target-serviceid` | — |
| updateExistingUserVehicleDetails | PUT | `fam/rsa/v5/vehicles/{vehicle-id}` | `x-fp-identifier-target-serviceid` | — |

## rtt (2)

| Method | HTTP | Path | Headers | Query |
|---|---|---|---|---|
| getHistoryEvents | GET | `/vsf/location/v5/rtt` | `x-fp-identifier-target-serviceid` | `includeLocEvents` |
| invoke | POST | `/vsf/location/v5/rtt` | `x-fp-identifier-target-serviceid` | — |

## safetyalerts (1)

| Method | HTTP | Path | Headers | Query |
|---|---|---|---|---|
| getSafetyAlerts | GET | `safety/fam/wm-sos/v5/safety-alerts` | `x-fp-identifier-target-serviceid` | — |

## screentime-schedules (3)

| Method | HTTP | Path | Headers | Query |
|---|---|---|---|---|
| getScreenTime | GET | `/frisco/parental-control/v1/app-usage` | — | `<@QueryMap HashMap<String,String> — dynamic query keys>` |
| putAppUsageStats | POST | `/frisco/parental-control/v5/web-app-activity` | `x-fp-identifier-target-serviceid` | — |
| updateSchedulerRunStatus | POST | `/vsf/tamper/v5/screenTimeScheduler` | `x-fp-identifier-target-serviceid` | — |

## screentime-schedules (ADJACENT — lives under feature/contentfilter, may belong to contentfilter/app-limits group) (3)

| Method | HTTP | Path | Headers | Query |
|---|---|---|---|---|
| deleteAppLimit | DELETE | `/parental-control/frisco/v7/app-limits` | `x-fp-identifier-target-serviceid` | `appLimitsId` |
| createAppLimit | POST | `/parental-control/frisco/v7/app-limits` | `x-fp-identifier-target-serviceid` | — |
| updateAppLimit | PUT | `/parental-control/frisco/v7/app-limits` | `x-fp-identifier-target-serviceid` | `appLimitsId` |

## screentime-schedules (ADJACENT — lives under feature/location/schedulealert, likely belongs to a location group) (4)

| Method | HTTP | Path | Headers | Query |
|---|---|---|---|---|
| deleteScheduledAlert | DELETE | `/vsf/location/v5/scheduled/alert/settings` | `x-fp-identifier-target-serviceid` | — |
| getScheduledAlerts | GET | `/vsf/location/v5/scheduled/alert/settings` | `x-fp-identifier-target-serviceid` | `eventType`, `profileRole`, `profileId` |
| postScheduleAlert | POST | `/vsf/location/v5/scheduled/alert/settings` | `x-fp-identifier-target-serviceid` | — |
| updateScheduledAlert | PUT | `/vsf/location/v5/scheduled/alert/settings` | `x-fp-identifier-target-serviceid` | — |

## screentime-schedules (ADJACENT — lives under feature/webAndApps, may belong to a web-and-apps group) (2)

| Method | HTTP | Path | Headers | Query |
|---|---|---|---|---|
| getCategories | GET | `/frisco/parental-control/v6/web-app-activity/categories` | `x-fp-identifier-target-serviceid` | `<@QueryMap HashMap<String,String> — dynamic query keys>` |
| getInsights | GET | `/frisco/parental-control/v6/web-app-activity/insights` | `x-fp-identifier-target-serviceid` | `timezone`, `usageType`, `categorySupported` |

## screentime-schedules (applimitasktime) (4)

| Method | HTTP | Path | Headers | Query |
|---|---|---|---|---|
| getAppTimeLimits | GET | `/parental-control/frisco/v7/app-limits` | `x-fp-identifier-target-serviceid` | `resolution`, `sz`, `timezone` |
| getAskTimeRequests | GET | `/parental-control/frisco/v8/app-limits/askTime` | `x-fp-identifier-target-serviceid` | `appLimitsId`, `screenTimeLimitId`, `packageName`, `timezone`, `additionalTimeId` |
| postAppTimeLimit | POST | `/parental-control/frisco/v7/app-limits/askTime` | `x-fp-identifier-target-serviceid` | — |
| acceptDeclineAskTimeRequest | PUT | `/parental-control/frisco/v7/app-limits` | `x-fp-identifier-target-serviceid` | `appLimitsId` |

## screentime-schedules (schedules) (4)

| Method | HTTP | Path | Headers | Query |
|---|---|---|---|---|
| deleteSchedule | DELETE | `parental-control/frisco/v6/schedules` | `x-fp-identifier-target-serviceid` | `schedule-id` |
| getSchedules | GET | `parental-control/frisco/v6/schedules` | `x-fp-identifier-target-serviceid` | — |
| postSchedule | POST | `parental-control/frisco/v6/schedules` | `x-fp-identifier-target-serviceid` | — |
| putSchedule | PUT | `parental-control/frisco/v6/schedules` | `x-fp-identifier-target-serviceid` | — |

## screentime-schedules (screentime) (6)

| Method | HTTP | Path | Headers | Query |
|---|---|---|---|---|
| deleteScreenTimeData | DELETE | `/parental-control/frisco/v7/screen-time-limits` | `x-fp-identifier-target-serviceid` | `screenTimeLimitId` |
| getScreenTimeData | GET | `/parental-control/frisco/v7/screen-time-limits` | `x-fp-identifier-target-serviceid` | `timezone` |
| postScreenTimeData | POST | `/parental-control/frisco/v7/screen-time-limits` | `x-fp-identifier-target-serviceid` | — |
| askForMoreScreenTime | POST | `/parental-control/frisco/v7/screen-time-limits/askTime` | `x-fp-identifier-target-serviceid` | — |
| putScreenTimeData | PUT | `/parental-control/frisco/v7/screen-time-limits` | `x-fp-identifier-target-serviceid` | — |
| actionOnScreenTimeData | PUT | `/parental-control/frisco/v7/screen-time-limits` | `x-fp-identifier-target-serviceid` | `requestType` |

## securitythreat (1)

| Method | HTTP | Path | Headers | Query |
|---|---|---|---|---|
| getThreats | GET | `/frisco/parental-control/v5/security-threats` | `x-fp-identifier-target-serviceid` | `(dynamic @QueryMap HashMap<String,String>)` |

## sendinvites (7)

| Method | HTTP | Path | Headers | Query |
|---|---|---|---|---|
| getAccountLines | GET | `/fam/userprofile-management/v5/accounts/lines` | `x-fp-identifier-target-serviceid` | — |
| getFeaturePermissions | GET | `/fam/userprofile-management/v5/accounts/userprofiles/featurepermissions` | `x-fp-identifier-target-serviceid` | — |
| replaceDevice | PATCH | `/account/fam/userprofile-management/v5/gizmo/device/replace` | `x-fp-identifier-target-serviceid` | — |
| sendStandaloneInvite | POST | `/account/fam/userprofile-management/v5/onboarding/device` | `x-fp-identifier-target-serviceid`, `x-pairing-required` | — |
| sendInvite | POST | `/account/fam/userprofile-management/v6/accounts/userprofiles` | `x-fp-identifier-target-serviceid` | — |
| updateFeaturePermissions | POST | `/fam/userprofile-management/v5/accounts/userprofiles/featurepermissions` | `x-fp-identifier-target-serviceid` | — |
| retryPairing | PUT | `/account/fam/userprofile-management/v5/gizmo/device/pairing` | `x-fp-identifier-target-serviceid` | — |

## setupwizard (2)

| Method | HTTP | Path | Headers | Query |
|---|---|---|---|---|
| getUserTasks | GET | `/account/fam/task-management/v5/userprofiles/tasks` | `x-fp-identifier-target-serviceid`, `x-fp-identifier-profileid` | — |
| updateUserTask | PATCH | `/account/fam/task-management/v5/userprofiles/tasks` | `x-fp-identifier-target-serviceid`, `x-fp-identifier-profileid` | — |

## soswatchme (11)

| Method | HTTP | Path | Headers | Query |
|---|---|---|---|---|
| getWatchMeSoSSessionInfo | GET | `fam/wm-sos/v5/sessions` | `x-fp-identifier-target-serviceid` | — |
| watchMeSosInsightSummaryList | GET | `fam/wm-sos/v5/sessions/insights/list` | `x-fp-identifier-target-serviceid` | — |
| getWatchMeSoSHistory | GET | `fam/wm-sos/v5/sessions/{session-id}/location/history` | `x-fp-identifier-target-serviceid` | — |
| watchMeSosAlerts | GET | `safety/fam/wm-sos/v6/sessions/insights/summary` | `x-fp-identifier-target-serviceid` | — |
| manageWmsPin | PATCH | `fam/wm-sos/v5/profiles` | `x-fp-identifier-target-serviceid` | — |
| updateSafeWalkProfile | PATCH | `fam/wm-sos/v5/profiles` | `x-fp-identifier-target-serviceid` | — |
| escalateToSoSRequest | PATCH | `fam/wm-sos/v5/sessions` | `x-fp-identifier-target-serviceid` | — |
| markSafeWatchMeSoSRequest | PATCH | `fam/wm-sos/v5/sessions` | `x-fp-identifier-target-serviceid` | — |
| extendSoSSession | PATCH | `fam/wm-sos/v5/sessions/extend` | `x-fp-identifier-target-serviceid` | — |
| postWmsOnboardProfile | POST | `fam/wm-sos/v5/profiles` | `x-fp-identifier-target-serviceid` | — |
| submitWatchMeRequest | POST | `fam/wm-sos/v5/sessions` | `x-fp-identifier-target-serviceid` | — |

## stepcounter (2)

| Method | HTTP | Path | Headers | Query |
|---|---|---|---|---|
| getChartStepsTracking | GET | `/comms/frisco/v1/stepsTracking` | `x-transaction-id`, `x-fp-identifier-target-serviceid`, `timezone`, `schedule-type` | — |
| setStepGoals | POST | `/fam/comms/v1/device/settings` | `x-fp-identifier-target-serviceid` | — |

## tamper (23)

| Method | HTTP | Path | Headers | Query |
|---|---|---|---|---|
| postAdminStatus | POST | `/vsf/tamper/v5/adminStatus` | `x-fp-identifier-target-serviceid` | — |
| postAdminStatus | POST | `/vsf/tamper/v5/adminStatus` | `x-fp-identifier-target-serviceid` | — |
| postAccessibilityStatus | POST | `/vsf/tamper/v5/androidAccessibility` | `x-fp-identifier-target-serviceid` | — |
| postAccessibilityStatus | POST | `/vsf/tamper/v5/androidAccessibility` | `x-fp-identifier-target-serviceid` | — |
| postBannerStatus | POST | `/vsf/tamper/v5/bannerStatus` | `x-fp-identifier-target-serviceid` | — |
| postBannerStatus | POST | `/vsf/tamper/v5/bannerStatus` | `x-fp-identifier-target-serviceid` | — |
| postBatteryUsage | POST | `/vsf/tamper/v5/batteryOptimization` | `x-fp-identifier-target-serviceid` | — |
| postBluetoothScan | POST | `/vsf/tamper/v5/bluetoothScan` | `x-fp-identifier-target-serviceid` | — |
| postHibernationStatus | POST | `/vsf/tamper/v5/hibernation` | `x-fp-identifier-target-serviceid` | — |
| postHibernationStatus | POST | `/vsf/tamper/v5/hibernation` | `x-fp-identifier-target-serviceid` | — |
| postHibernationStatus | POST | `/vsf/tamper/v5/hibernation` | `x-fp-identifier-target-serviceid` | — |
| postNotificationStatus | POST | `/vsf/tamper/v5/notificationStatus` | `x-fp-identifier-target-serviceid` | — |
| postParentalControlsRemoved | POST | `/vsf/tamper/v5/parentalControlsRemoved` | `x-fp-identifier-target-serviceid` | — |
| postParentalControlsRemoved | POST | `/vsf/tamper/v5/parentalControlsRemoved` | `x-fp-identifier-target-serviceid` | — |
| postPhysicalActivityStatus | POST | `/vsf/tamper/v5/physicalActivityStatus` | `x-fp-identifier-target-serviceid` | — |
| postPowerSaving | POST | `/vsf/tamper/v5/powerSaving` | `x-fp-identifier-target-serviceid` | — |
| updateScreenTimeTamperStatus | POST | `/vsf/tamper/v5/screenTimeStatus` | `x-fp-identifier-target-serviceid` | — |
| postCallOnlyModeStatus | POST | `comms/vsf/tamper/v5/callOnlyMode` | `x-fp-identifier-target-serviceid` | — |
| postCallOnlyModeStatus | POST | `comms/vsf/tamper/v5/callOnlyMode` | `x-fp-identifier-target-serviceid` | — |
| postScreenTimeCallOnlyModeStatus | POST | `comms/vsf/tamper/v5/screenTimeCallOnlyMode` | `x-fp-identifier-target-serviceid` | — |
| postScreenTimeCallOnlyModeStatus | POST | `comms/vsf/tamper/v5/screenTimeCallOnlyMode` | `x-fp-identifier-target-serviceid` | — |
| putTamperInstructions | PUT | `/fam/userprofile-management/v5/tamper/instructions` | `x-fp-identifier-target-serviceid` | — |
| putTamperInstructions | PUT | `/fam/userprofile-management/v5/tamper/instructions` | `x-fp-identifier-target-serviceid` | — |

## todo (4)

| Method | HTTP | Path | Headers | Query |
|---|---|---|---|---|
| deleteTodo | DELETE | `/comms/fam/v1/todos` | `x-fp-identifier-target-serviceid` | `todoId` |
| invoke (getTodos) | GET | `/comms/fam/v1/todos` | `x-fp-identifier-target-serviceid`, `timezone` | — |
| createTodo | POST | `/comms/fam/v1/todos` | `x-fp-identifier-target-serviceid`, `x-transaction-id` | — |
| updateTodo | PUT | `/comms/fam/v1/todos` | `x-fp-identifier-target-serviceid`, `x-transaction-id` | — |

## videocalling (3)

| Method | HTTP | Path | Headers | Query |
|---|---|---|---|---|
| callManage | POST | `/comms/fam/v1/webrtc/callManage` | `x-fp-identifier-target-serviceid` | — |
| initCall | POST | `/comms/fam/v1/webrtc/initCall` | `x-fp-identifier-target-serviceid` | — |
| initCallAnswer | POST | `/comms/fam/v1/webrtc/initCallAnswer` | `x-fp-identifier-target-serviceid` | — |

## vpnstatus (1)

| Method | HTTP | Path | Headers | Query |
|---|---|---|---|---|
| getWebAppVisibility | GET | `/frisco/parental-control/v5/web-app-activity/visibility` | `x-fp-identifier-target-serviceid` | `vpn-retry-type` |

## vpntamper (1)

| Method | HTTP | Path | Headers | Query |
|---|---|---|---|---|
| updateVpnTamperStatus | POST | `/vsf/tamper/v5/vpn` | `x-fp-identifier-target-serviceid` | — |

## wearable (5)

| Method | HTTP | Path | Headers | Query |
|---|---|---|---|---|
| resendInvite | PATCH | `/fam/userprofile-management/v5/userprofiles/services/invites` | `x-fp-identifier-target-serviceid` | — |
| confirmWatchPairing | POST | `/auth/frisco/frisco-iam-wearable-auth/v1/watch/auth/verify` | `x-fp-identifier-target-serviceid`, `x-trace-transaction-id` | — |
| watchAuth | POST | `/frisco/frisco-iam-device-auth/v5/watch/auth/token` | `x-trace-transaction-id` | — |
| notifyGuardianFromDependantWatch | POST | `/safety/fam/wearable/v6/locations/notification` | `x-fp-identifier-target-serviceid`, `x-transaction-id` | — |
| onboardWearableWatch | POST | `account/fam/userprofile-management/v6/accounts/userprofiles` | `x-fp-identifier-target-serviceid`, `x-trace-transaction-id` | — |

## webAndApps (7)

| Method | HTTP | Path | Headers | Query |
|---|---|---|---|---|
| getWebsites | GET | `/frisco/parental-control/v5/web-app-activity/websites` | `x-fp-identifier-target-serviceid` | `<QueryMap: HashMap<String,String>>` |
| getWebsites | GET | `/frisco/parental-control/v5/website` | `x-fp-identifier-target-serviceid` | — |
| getCategories | GET | `/frisco/parental-control/v6/web-app-activity/categories` | `x-fp-identifier-target-serviceid` | `<QueryMap: HashMap<String,String>>` |
| getWebsiteDetails | GET | `/frisco/parental-control/v6/web-app-activity/details/website` | `x-fp-identifier-target-serviceid` | `<QueryMap: Map<String,String>>` |
| getInsights | GET | `/frisco/parental-control/v6/web-app-activity/insights` | `x-fp-identifier-target-serviceid` | `timezone`, `usageType`, `categorySupported` |
| getWebsite | GET | `/frisco/parental-control/v6/web-app-activity/websites` | `x-fp-identifier-target-serviceid` | `<QueryMap: HashMap<String,String> (nullable)>` |
| getAppUsageDetails | GET | `/parental-control/frisco/v7/web-app-activity/apps` | `x-fp-identifier-target-serviceid` | `<QueryMap: HashMap<String,String>>` |

## website-block (5)

| Method | HTTP | Path | Headers | Query |
|---|---|---|---|---|
| deleteWebsite | DELETE | `/frisco/parental-control/v5/website` | `x-fp-identifier-target-serviceid` | `profileDomainId` |
| disableSafeSearch | DELETE | `/frisco/parental-control/v5/website/safesearch` | `x-fp-identifier-target-serviceid` | — |
| postWebsite | POST | `/frisco/parental-control/v5/website` | `x-fp-identifier-target-serviceid` | — |
| postWebsites | POST | `/frisco/parental-control/v5/website` | `x-fp-identifier-target-serviceid` | — |
| enableSafeSearch | POST | `/frisco/parental-control/v5/website/safesearch` | `x-fp-identifier-target-serviceid` | — |

## whats-new (1)

| Method | HTTP | Path | Headers | Query |
|---|---|---|---|---|
| getWhatsNew | GET | `/account/fam/userprofile-management/v1/releases` | `@HeaderMap (dynamic)` | — |

## whocanviewwebandapp (1)

| Method | HTTP | Path | Headers | Query |
|---|---|---|---|---|
| getParentalControlFeaturePermissions | GET | `/vsf/commsplatform/v5/view/featurePermissions` | `x-fp-identifier-target-serviceid` | `featureGroup` |
