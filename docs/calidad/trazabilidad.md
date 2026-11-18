# Trazabilidad de requisitos a pruebas (spec 016, T003)

Generado con `python3 -I scripts/trace.py`. Una prueba cuenta si cita el ID en su nombre, su comentario o su cuerpo. «Sin prueba» no siempre es un hueco: algunos requisitos se verifican por despliegue, documentación o revisión; esos casos se revisan a mano en `deuda.md`.

**Resumen:** 156 de 287 IDs con al menos una prueba que los cita.

## 003-cuenta-autenticacion (11/22 con prueba)

| ID | Pruebas |
|---|---|
| RF-01 | **sin prueba** |
| RF-02 | **sin prueba** |
| RF-03 | **sin prueba** |
| RF-04 | `internal/auth/usecase/auth_usecase_test.go:TestLogin` |
| RF-05 | **sin prueba** |
| RF-06 | **sin prueba** |
| RF-07 | `internal/auth/adapter/http/handler_test.go:TestAuthRateLimit` |
| RF-08 | **sin prueba** |
| RF-09 | **sin prueba** |
| RF-10 | **sin prueba** |
| RF-11 | **sin prueba** |
| RF-12 | **sin prueba** |
| RF-13 | `internal/auth/adapter/http/handler_test.go:TestRequireRoleAndActive` |
| RNF-01 | `internal/auth/adapter/http/handler_test.go:TestDomainErrorsMapToStatus` |
| RNF-02 | **sin prueba** |
| CA-01 | `internal/auth/adapter/http/flow_integration_test.go:TestIntegrationSessionFlow` |
| CA-02 | `internal/auth/adapter/http/flow_integration_test.go:TestIntegrationSessionFlow`<br>`internal/auth/usecase/auth_usecase_test.go:TestRegisterDuplicateEmailIgnoresCase` |
| CA-03 | `internal/auth/adapter/http/flow_integration_test.go:TestIntegrationSessionFlow`<br>`internal/auth/adapter/http/rotation_integration_test.go:TestIntegrationJWTRotation`<br>`internal/auth/usecase/session_usecase_test.go:TestRefreshReuseRevokesFamily` |
| CA-04 | `internal/auth/adapter/http/flow_integration_test.go:TestIntegrationExpiredAccessToken`<br>`internal/auth/adapter/http/flow_integration_test.go:TestMain`<br>`internal/auth/adapter/http/handler_test.go:TestAuthenticate` |
| CA-05 | `internal/auth/adapter/http/flow_integration_test.go:TestIntegrationSessionFlow`<br>`internal/auth/usecase/auth_usecase_test.go:TestLoginDeactivatedAccount`<br>`internal/auth/usecase/session_usecase_test.go:TestDeactivate` |
| CA-06 | `internal/auth/adapter/http/flow_integration_test.go:TestIntegrationSessionFlow`<br>`internal/auth/adapter/http/handler_test.go:TestAuthRateLimit`<br>`internal/auth/adapter/http/handler_test.go:TestRequireRoleAndActive` |
| CA-07 | `internal/auth/adapter/http/flow_integration_test.go:TestIntegrationSessionFlow`<br>`internal/auth/adapter/http/flow_integration_test.go:TestMain`<br>`internal/auth/adapter/http/handler_test.go:TestRequireRoleAndActive` |

## 004-imagenes (10/15 con prueba)

| ID | Pruebas |
|---|---|
| RF-01 | **sin prueba** |
| RF-02 | `internal/images/domain/image_test.go:TestPurposeAndDelivery`<br>`internal/images/usecase/usecase_test.go:TestSignature` |
| RF-03 | `internal/images/adapter/http/images_integration_test.go:TestIntegrationTamperedUploadRejected`<br>`internal/images/usecase/usecase_test.go:TestSignature` |
| RF-04 | `internal/images/adapter/http/handler_test.go:TestSignatureRateLimitPerUser` |
| RF-05 | **sin prueba** |
| RF-06 | **sin prueba** |
| RF-07 | **sin prueba** |
| RF-08 | `internal/images/adapter/http/images_integration_test.go:TestIntegrationResolveRejects`<br>`internal/images/usecase/usecase_test.go:TestResolveUnavailable` |
| RNF-01 | `internal/images/adapter/http/images_integration_test.go:TestIntegrationUploadFlow` |
| RNF-02 | **sin prueba** |
| CA-01 | `internal/images/adapter/http/handler_test.go:TestSignatureOK`<br>`internal/images/adapter/http/images_integration_test.go:TestIntegrationTamperedUploadRejected`<br>`internal/images/adapter/http/images_integration_test.go:TestIntegrationUploadFlow`<br>`internal/images/usecase/usecase_test.go:TestSignature` |
| CA-02 | `internal/images/adapter/http/images_integration_test.go:TestIntegrationResolveRejects`<br>`internal/images/usecase/usecase_test.go:TestResolveRejects` |
| CA-03 | `internal/images/adapter/http/images_integration_test.go:TestIntegrationResolveRejects`<br>`internal/images/usecase/usecase_test.go:TestResolveRejects` |
| CA-04 | `internal/images/adapter/cloudinary/urlbuilder_test.go:TestURLsAuthenticated`<br>`internal/images/adapter/http/images_integration_test.go:TestIntegrationEvidenceSignedURL` |
| CA-05 | `internal/images/adapter/http/handler_test.go:TestSignatureErrors`<br>`internal/images/adapter/http/images_integration_test.go:TestIntegrationUploadFlow` |

## 005-objetos (10/21 con prueba)

| ID | Pruebas |
|---|---|
| RF-01 | **sin prueba** |
| RF-02 | **sin prueba** |
| RF-03 | **sin prueba** |
| RF-04 | `internal/items/adapter/mongo/item_repo_integration_test.go:TestIntegrationItemRepoCRUD` |
| RF-05 | **sin prueba** |
| RF-06 | **sin prueba** |
| RF-07 | **sin prueba** |
| RF-08 | **sin prueba** |
| RF-09 | **sin prueba** |
| RF-10 | **sin prueba** |
| RF-11 | **sin prueba** |
| RF-12 | `internal/items/adapter/http/handler_test.go:TestCreateAuthAndSuspended` |
| RNF-01 | **sin prueba** |
| RNF-02 | `internal/items/adapter/http/handler_test.go:TestCreate` |
| CA-01 | `internal/items/adapter/http/items_integration_test.go:TestIntegrationItemLifecycle`<br>`internal/items/domain/domain_test.go:TestValidateErrors` |
| CA-02 | `internal/items/adapter/http/items_integration_test.go:TestIntegrationPagination`<br>`internal/items/domain/domain_test.go:TestSuggestPrice`<br>`internal/items/usecase/usecase_test.go:TestSuggestPriceAndCategories` |
| CA-03 | `internal/items/adapter/http/items_integration_test.go:TestIntegrationItemLifecycle`<br>`internal/items/domain/domain_test.go:TestRoundCoord`<br>`internal/items/usecase/usecase_test.go:TestCreate` |
| CA-04 | `internal/items/adapter/http/items_integration_test.go:TestIntegrationItemLifecycle`<br>`internal/items/adapter/http/items_integration_test.go:TestMain`<br>`internal/items/usecase/usecase_test.go:TestCreateErrors` |
| CA-05 | `internal/items/adapter/http/items_integration_test.go:TestIntegrationItemLifecycle`<br>`internal/items/domain/domain_test.go:TestOwnerChecks`<br>`internal/items/usecase/usecase_test.go:TestUpdateErrors` |
| CA-06 | `internal/items/adapter/http/items_integration_test.go:TestIntegrationItemLifecycle`<br>`internal/items/domain/domain_test.go:TestVisibility`<br>`internal/items/usecase/usecase_test.go:TestGetVisibility` |
| CA-07 | `internal/items/adapter/http/items_integration_test.go:TestIntegrationItemLifecycle`<br>`internal/items/adapter/http/items_integration_test.go:TestMain` |

## 006-busqueda (6/16 con prueba)

| ID | Pruebas |
|---|---|
| RF-01 | **sin prueba** |
| RF-02 | `internal/search/adapter/mongo/search_repo_integration_test.go:TestIntegrationExclusionsCA05` |
| RF-03 | `internal/search/adapter/mongo/availability_integration_test.go:TestIntegrationAvailabilityCA03` |
| RF-04 | **sin prueba** |
| RF-05 | **sin prueba** |
| RF-06 | **sin prueba** |
| RF-07 | **sin prueba** |
| RF-08 | **sin prueba** |
| RF-09 | `internal/search/adapter/mongo/search_perf_test.go:TestPerfSearch` |
| RNF-01 | **sin prueba** |
| CA-01 | **sin prueba** |
| CA-02 | **sin prueba** |
| CA-03 | `internal/search/adapter/mongo/availability_integration_test.go:TestIntegrationAvailabilityCA03` |
| CA-04 | **sin prueba** |
| CA-05 | `internal/search/adapter/mongo/search_repo_integration_test.go:TestIntegrationExclusionsCA05` |
| CA-06 | `internal/search/adapter/http/handler_test.go:TestSearchErrorsAndRoutes`<br>`internal/search/domain/domain_test.go:TestParseErrors`<br>`internal/search/usecase/search_test.go:TestSearchErrors` |

## 007-solicitudes-reservas (11/24 con prueba)

| ID | Pruebas |
|---|---|
| RF-01 | **sin prueba** |
| RF-02 | **sin prueba** |
| RF-03 | **sin prueba** |
| RF-04 | **sin prueba** |
| RF-05 | `internal/bookings/usecase/usecase_read_test.go:TestExpireDueCA06`<br>`internal/bookings/usecase/usecase_read_test.go:TestGetShowsHandoverStagesOnlyToParties` |
| RF-06 | **sin prueba** |
| RF-07 | **sin prueba** |
| RF-08 | **sin prueba** |
| RF-09 | **sin prueba** |
| RF-10 | **sin prueba** |
| RF-11 | **sin prueba** |
| RF-12 | **sin prueba** |
| RF-13 | **sin prueba** |
| RF-14 | `internal/bookings/adapter/http/flow_integration_test.go:TestIntegrationBookingFlow`<br>`internal/bookings/adapter/mongo/accept_tx_integration_test.go:TestIntegrationActivityCheckers` |
| RNF-01 | `internal/bookings/adapter/http/handover_dto_test.go:TestToBookingIncludesHandoverData`<br>`internal/bookings/adapter/mongo/accept_tx_integration_test.go:TestIntegrationAcceptConcurrencyCA05` |
| RNF-02 | **sin prueba** |
| CA-01 | `internal/bookings/adapter/http/flow_integration_test.go:TestIntegrationBookingFlow` |
| CA-02 | `internal/bookings/adapter/http/flow_integration_test.go:TestIntegrationBookingFlow` |
| CA-03 | `internal/bookings/adapter/http/flow_integration_test.go:TestIntegrationBookingFlow`<br>`internal/bookings/usecase/usecase_create_test.go:TestCreateCA07Snapshot` |
| CA-04 | `internal/bookings/adapter/http/flow_integration_test.go:TestIntegrationBookingFlow`<br>`internal/bookings/domain/statemachine_test.go:TestNewRequest`<br>`internal/bookings/usecase/usecase_create_test.go:TestQuoteCA01` |
| CA-05 | `internal/bookings/adapter/http/handler_test.go:TestActions`<br>`internal/bookings/adapter/mongo/accept_tx_integration_test.go:TestIntegrationAcceptConcurrencyCA05`<br>`internal/bookings/usecase/usecase_flow_test.go:TestAccept` |
| CA-06 | `internal/bookings/domain/statemachine_test.go:TestExpiry` |
| CA-07 | `internal/bookings/adapter/http/flow_integration_test.go:TestIntegrationBookingFlow` |
| CA-08 | `internal/bookings/adapter/http/flow_integration_test.go:TestIntegrationBookingFlow`<br>`internal/bookings/adapter/mongo/accept_tx_integration_test.go:TestIntegrationActivityCheckers`<br>`internal/bookings/usecase/usecase_flow_test.go:TestRejectAndCancel` |

## 008-chat (12/20 con prueba)

| ID | Pruebas |
|---|---|
| RF-01 | **sin prueba** |
| RF-02 | **sin prueba** |
| RF-03 | **sin prueba** |
| RF-04 | **sin prueba** |
| RF-05 | **sin prueba** |
| RF-06 | **sin prueba** |
| RF-07 | `internal/chat/domain/domain_test.go:TestCanWrite`<br>`internal/chat/usecase/usecase_test.go:TestSendRulesCA02CA03CA04CA05` |
| RF-08 | `internal/chat/adapter/http/handler_test.go:TestList`<br>`internal/chat/adapter/mongo/message_reader_integration_test.go:TestMessageReaderReference` |
| RF-09 | **sin prueba** |
| RF-10 | `internal/chat/usecase/usecase_test.go:TestSendRulesCA02CA03CA04CA05` |
| RF-11 | `internal/chat/adapter/http/handler_test.go:TestRateLimit`<br>`internal/chat/adapter/http/handler_test.go:TestSend` |
| RF-12 | `internal/chat/adapter/http/chat_integration_test.go:TestIntegrationChatFlow`<br>`internal/chat/usecase/usecase_test.go:TestSendAndUnreadCA06` |
| RNF-01 | `internal/chat/adapter/http/chat_integration_test.go:TestIntegrationChatFlow`<br>`internal/chat/adapter/http/chat_integration_test.go:TestIntegrationPollingPerformance` |
| RNF-02 | **sin prueba** |
| CA-01 | `internal/chat/adapter/http/chat_integration_test.go:TestIntegrationChatFlow` |
| CA-02 | `internal/chat/adapter/http/chat_integration_test.go:TestIntegrationChatFlow`<br>`internal/chat/adapter/http/handler_test.go:TestMessages`<br>`internal/chat/usecase/usecase_test.go:TestSendRulesCA02CA03CA04CA05` |
| CA-03 | `internal/chat/adapter/http/chat_integration_test.go:TestIntegrationChatFlow`<br>`internal/chat/adapter/http/handler_test.go:TestSend`<br>`internal/chat/usecase/usecase_test.go:TestSendRulesCA02CA03CA04CA05` |
| CA-04 | `internal/chat/adapter/http/chat_integration_test.go:TestIntegrationChatFlow`<br>`internal/chat/adapter/http/handler_test.go:TestSend`<br>`internal/chat/usecase/usecase_test.go:TestSendRulesCA02CA03CA04CA05` |
| CA-05 | `internal/chat/adapter/http/chat_integration_test.go:TestIntegrationChatFlow`<br>`internal/chat/adapter/http/handler_test.go:TestSend`<br>`internal/chat/domain/domain_test.go:TestNormalizeBody`<br>`internal/chat/usecase/usecase_test.go:TestSendRulesCA02CA03CA04CA05` |
| CA-06 | `internal/chat/adapter/http/chat_integration_test.go:TestIntegrationChatFlow`<br>`internal/chat/adapter/mongo/message_reader_integration_test.go:TestMessageReaderReference`<br>`internal/chat/adapter/mongo/repo_integration_test.go:TestIntegrationUnreadCounters` |

## 009-entrega-devolucion-pagos (19/25 con prueba)

| ID | Pruebas |
|---|---|
| RF-01 | `internal/handover/domain/domain_test.go:TestRecordBumpsVersionAndDropsQR` |
| RF-02 | **sin prueba** |
| RF-03 | **sin prueba** |
| RF-04 | **sin prueba** |
| RF-05 | `internal/handover/usecase/usecase_confirm_test.go:TestGetStage` |
| RF-06 | **sin prueba** |
| RF-07 | `internal/payments/usecase/usecase_test.go:TestPaymentDoesNotChangeBookingStatus`<br>`internal/payments/usecase/usecase_test.go:TestReportSurvivesNotifierFailure` |
| RF-08 | `internal/payments/domain/domain_test.go:TestPlanReport`<br>`internal/payments/usecase/usecase_test.go:TestPaymentErrors` |
| RF-09 | `internal/payments/usecase/late_test.go:TestDetectLateMarksAndNotifies` |
| RF-10 | `internal/payments/domain/domain_test.go:TestAdjustment` |
| RF-11 | **sin prueba** |
| RF-12 | `internal/handover/adapter/http/handover_integration_test.go:TestIntegrationDeliveryAndReturnFlow`<br>`internal/handover/usecase/usecase_confirm_test.go:TestDeliveryThenReturnFlow` |
| RF-13 | `internal/handover/adapter/http/handover_integration_test.go:TestIntegrationDeliveryAndReturnFlow`<br>`internal/handover/adapter/mongo/stage_repo_integration_test.go:TestConfirmRejectsStaleQR`<br>`internal/handover/usecase/usecase_evidence_test.go:TestRecordEvidenceAfterConfirmationIsRejected` |
| RNF-01 | `internal/handover/adapter/http/handler_test.go:TestIssueQR`<br>`internal/handover/adapter/http/handover_integration_test.go:TestIntegrationDeliveryAndReturnFlow`<br>`internal/handover/usecase/usecase_evidence_test.go:TestIssueQR` |
| RNF-02 | `internal/handover/adapter/http/handover_integration_test.go:TestIntegrationConcurrentConfirmationsOnlyOneWins`<br>`internal/handover/adapter/http/handover_integration_test.go:TestIntegrationConfirmWindowAndStatus`<br>`internal/handover/adapter/mongo/stage_repo_integration_test.go:TestConfirmConcurrentExactlyOneWins`<br>`internal/handover/adapter/mongo/stage_repo_integration_test.go:TestConfirmRejectsStaleQR` (+1) |
| RNF-03 | **sin prueba** |
| CA-01 | `internal/handover/adapter/http/handover_integration_test.go:TestIntegrationDeliveryAndReturnFlow`<br>`internal/handover/adapter/mongo/stage_repo_integration_test.go:TestConfirmDelivery`<br>`internal/handover/usecase/usecase_confirm_test.go:TestDeliveryThenReturnFlow` |
| CA-02 | `internal/handover/adapter/http/handover_integration_test.go:TestIntegrationQRExpiryRenewalAndEvidenceChange`<br>`internal/handover/adapter/mongo/stage_repo_integration_test.go:TestConfirmRejectsStaleQR`<br>`internal/handover/domain/domain_test.go:TestStateOfAndVerifyQR`<br>`internal/handover/usecase/usecase_confirm_test.go:TestConfirmQRExpiredThenRenew` |
| CA-03 | `internal/handover/adapter/http/handover_integration_test.go:TestIntegrationDeliveryAndReturnFlow`<br>`internal/handover/adapter/mongo/stage_repo_integration_test.go:TestConfirmDelivery`<br>`internal/handover/domain/domain_test.go:TestStateOfAndVerifyQR`<br>`internal/handover/usecase/usecase_confirm_test.go:TestConfirmUsedQR` |
| CA-04 | `internal/handover/adapter/http/handover_integration_test.go:TestIntegrationQRExpiryRenewalAndEvidenceChange`<br>`internal/handover/adapter/mongo/stage_repo_integration_test.go:TestConfirmRejectsStaleQR`<br>`internal/handover/adapter/mongo/stage_repo_integration_test.go:TestSaveEvidenceAndQR`<br>`internal/handover/domain/domain_test.go:TestStateOfAndVerifyQR` (+1) |
| CA-05 | `internal/handover/adapter/http/handover_integration_test.go:TestIntegrationDeliveryAndReturnFlow`<br>`internal/handover/usecase/usecase_confirm_test.go:TestDeliveryThenReturnFlow` |
| CA-06 | `internal/payments/adapter/http/payments_integration_test.go:TestIntegrationPaymentFlow`<br>`internal/payments/domain/domain_test.go:TestPlanConfirm`<br>`internal/payments/usecase/usecase_test.go:TestReportThenConfirm` |
| CA-07 | `internal/handover/usecase/usecase_confirm_test.go:TestReturnRecordsLateAdjustment`<br>`internal/payments/adapter/http/payments_integration_test.go:TestIntegrationLateReturnAdjustment`<br>`internal/payments/domain/domain_test.go:TestAdjustment`<br>`internal/payments/domain/domain_test.go:TestLateDaysTable` |
| CA-08 | `internal/payments/adapter/http/payments_integration_test.go:TestIntegrationDetectorMarksLateAndAffected`<br>`internal/payments/domain/domain_test.go:TestAffects`<br>`internal/payments/usecase/late_test.go:TestDetectLateMarksAffectedBooking` |
| CA-09 | `internal/handover/adapter/http/handover_integration_test.go:TestIntegrationDeliveryAndReturnFlow`<br>`internal/handover/usecase/usecase_confirm_test.go:TestGetStage` |

## 010-calificaciones-incidencias (14/21 con prueba)

| ID | Pruebas |
|---|---|
| RF-01 | **sin prueba** |
| RF-02 | `internal/ratings/adapter/mongo/rating_repo_integration_test.go:TestIntegrationCreateUpdatesProfileAtomically` |
| RF-03 | **sin prueba** |
| RF-04 | `internal/ratings/adapter/mongo/rating_repo_integration_test.go:TestIntegrationListByRateeNewestFirstWithCursor`<br>`internal/ratings/usecase/usecase_test.go:TestListPagesNewestFirstWithNames` |
| RF-05 | **sin prueba** |
| RF-06 | `internal/incidents/domain/domain_test.go:TestReasonStatusOutcomeValid`<br>`internal/incidents/usecase/usecase_test.go:TestOpenCreatesIncidentAndNotifiesReported` |
| RF-07 | `internal/incidents/adapter/mongo/incident_repo_integration_test.go:TestIntegrationListByPartyBothRolesWithCursor`<br>`internal/incidents/usecase/usecase_test.go:TestGetVisibility`<br>`internal/incidents/usecase/usecase_test.go:TestListMinePagesBothRoles` |
| RF-08 | `internal/incidents/adapter/mongo/incident_repo_integration_test.go:TestIntegrationListAllOldestFirstByStatus`<br>`internal/incidents/usecase/admin_test.go:TestAdminListDefaultsToOpenOldestFirstWithAllNotes`<br>`internal/incidents/usecase/admin_test.go:TestCloseNotifiesBothPartiesAndRecordsOutcome` |
| RF-09 | **sin prueba** |
| RF-10 | **sin prueba** |
| RF-11 | **sin prueba** |
| RNF-01 | `internal/incidents/adapter/http/trust_integration_test.go:TestIntegrationRatingsFlow`<br>`internal/ratings/adapter/mongo/rating_repo_integration_test.go:TestIntegrationConcurrentDuplicatesOneWins` |
| RNF-02 | **sin prueba** |
| CA-01 | `internal/incidents/adapter/http/trust_integration_test.go:TestIntegrationRatingsFlow`<br>`internal/ratings/adapter/http/handler_test.go:TestRateCreated`<br>`internal/ratings/adapter/mongo/rating_repo_integration_test.go:TestIntegrationCreateUpdatesProfileAtomically`<br>`internal/ratings/usecase/usecase_test.go:TestRateSavesAndNotifiesTheOtherParty` |
| CA-02 | `internal/incidents/adapter/http/trust_integration_test.go:TestIntegrationRatingsFlow`<br>`internal/ratings/adapter/http/handler_test.go:TestRateDomainErrorsMapToStatus`<br>`internal/ratings/usecase/usecase_test.go:TestRateRejections` |
| CA-03 | `internal/incidents/adapter/http/trust_integration_test.go:TestIntegrationRatingsFlow`<br>`internal/ratings/adapter/http/handler_test.go:TestRateDomainErrorsMapToStatus`<br>`internal/ratings/adapter/mongo/rating_repo_integration_test.go:TestIntegrationDuplicateRatingLeavesProfileUntouched`<br>`internal/ratings/usecase/usecase_test.go:TestRateTwiceIsConflict` |
| CA-04 | `internal/incidents/adapter/http/handler_test.go:TestOpenCreated`<br>`internal/incidents/adapter/http/trust_integration_test.go:TestIntegrationIncidentLifecycleAndSanction`<br>`internal/incidents/usecase/admin_test.go:TestCloseDoesNotChangeTheBooking`<br>`internal/incidents/usecase/usecase_test.go:TestOpenCreatesIncidentAndNotifiesReported` |
| CA-05 | `internal/incidents/adapter/http/handler_test.go:TestAdminRoutesRejectNonAdmin`<br>`internal/incidents/adapter/http/trust_integration_test.go:TestIntegrationIncidentLifecycleAndSanction`<br>`internal/sanctions/adapter/http/handler_test.go:TestRequiresSessionAndAdminRole`<br>`internal/sanctions/usecase/apply_test.go:TestApplySuspensionSuspendsThenSavesThenNotifies` |
| CA-06 | `internal/incidents/adapter/http/trust_integration_test.go:TestIntegrationIncidentLifecycleAndSanction`<br>`internal/sanctions/adapter/http/handler_test.go:TestApplySuspension`<br>`internal/sanctions/usecase/apply_test.go:TestApplySuspensionSuspendsThenSavesThenNotifies` |
| CA-07 | `internal/incidents/adapter/http/handler_test.go:TestGetPassesViewerRoleAndHidesInternalFlag`<br>`internal/incidents/adapter/http/trust_integration_test.go:TestIntegrationIncidentLifecycleAndSanction`<br>`internal/incidents/adapter/mongo/incident_repo_integration_test.go:TestIntegrationCloseOnlyOnceAndBlocksNotes`<br>`internal/incidents/usecase/admin_test.go:TestCloseNotifiesBothPartiesAndRecordsOutcome` (+2) |
| CA-08 | `internal/incidents/adapter/http/trust_integration_test.go:TestIntegrationIncidentLifecycleAndSanction`<br>`internal/incidents/adapter/mongo/incident_repo_integration_test.go:TestIntegrationCountOpenIgnoresClosedAndOtherBookings`<br>`internal/incidents/domain/domain_test.go:TestCanOpen`<br>`internal/incidents/usecase/usecase_test.go:TestOpenFourthIncidentFails` |

## 011-notificaciones-moderacion (18/21 con prueba)

| ID | Pruebas |
|---|---|
| RF-01 | **sin prueba** |
| RF-02 | `internal/notify/templates_test.go:TestEveryRF02TypeHasSpanishTemplate`<br>`internal/notify/templates_test.go:TestNormalize` |
| RF-03 | **sin prueba** |
| RF-04 | `internal/notifications/adapter/http/push_integration_test.go:TestIntegrationTokenReassignedToNewUser`<br>`internal/notifications/adapter/mongo/repo_integration_test.go:TestTokenUpsertReassignsToNewUser` |
| RF-05 | `internal/notifications/adapter/http/push_integration_test.go:TestIntegrationBookingRequestCreatesNoticeAndPush`<br>`internal/notifications/adapter/http/push_integration_test.go:TestIntegrationDeletedAccountGetsNoNoticeNorPush`<br>`internal/notifications/worker/worker_test.go:TestBatchesNeverExceed100`<br>`internal/notify/service_test.go:TestNotifySkipsDeletedOrMissingAccounts` |
| RF-06 | `internal/notifications/adapter/http/push_integration_test.go:TestIntegrationBookingRequestCreatesNoticeAndPush`<br>`internal/notifications/worker/worker_test.go:TestSendsOneMessagePerTokenWithData` |
| RF-07 | `internal/moderation/filter_test.go:TestFilterBlocksEachCategory` |
| RF-08 | `internal/reports/adapter/http/moderation_integration_test.go:TestIntegrationChatForbiddenMessageAndReportRules`<br>`internal/reports/adapter/http/moderation_integration_test.go:TestIntegrationReportedItemIsHiddenFromSearch`<br>`internal/reports/adapter/mongo/report_repo_integration_test.go:TestDuplicateReturnsExisting`<br>`internal/reports/usecase/usecase_test.go:TestCreateDuplicateReturnsExisting` |
| RF-09 | `internal/reports/adapter/http/moderation_integration_test.go:TestIntegrationChatForbiddenMessageAndReportRules`<br>`internal/reports/usecase/usecase_test.go:TestCreateTargets` |
| RF-10 | `internal/reports/adapter/http/moderation_integration_test.go:TestIntegrationReportedItemIsHiddenFromSearch`<br>`internal/reports/usecase/usecase_test.go:TestResolveHideItem` |
| RF-11 | `internal/reports/adapter/http/moderation_integration_test.go:TestIntegrationReportedItemIsHiddenFromSearch`<br>`internal/reports/adapter/mongo/report_repo_integration_test.go:TestResolveKeepsDecisionAndAdmin`<br>`internal/reports/usecase/usecase_test.go:TestResolveHideItem` |
| RNF-01 | `internal/notifications/adapter/mongo/repo_integration_test.go:TestIndexesIncludeTTL90Days` |
| RNF-02 | `internal/moderation/filter_test.go:TestFilterIsFast` |
| RNF-03 | **sin prueba** |
| CA-01 | `internal/notifications/adapter/http/push_integration_test.go:TestIntegrationBookingRequestCreatesNoticeAndPush`<br>`internal/notify/service_test.go:TestNotifySavesAndPushesWithTemplate` |
| CA-02 | `internal/notifications/adapter/http/push_integration_test.go:TestIntegrationDeviceNotRegisteredDeletesTokenAndBusinessSucceeds`<br>`internal/notifications/adapter/mongo/repo_integration_test.go:TestDeleteTokenAnyOwner`<br>`internal/notifications/worker/worker_test.go:TestDeviceNotRegisteredDeletesToken` |
| CA-03 | `internal/moderation/filter_test.go:TestFilterBlocksEachCategory`<br>`internal/reports/adapter/http/moderation_integration_test.go:TestIntegrationForbiddenItemTitleIsRejected` |
| CA-04 | `internal/reports/adapter/http/moderation_integration_test.go:TestIntegrationChatForbiddenMessageAndReportRules` |
| CA-05 | `internal/reports/adapter/http/moderation_integration_test.go:TestIntegrationReportedItemIsHiddenFromSearch`<br>`internal/reports/usecase/usecase_test.go:TestResolveHideItem` |
| CA-06 | `internal/reports/adapter/http/moderation_integration_test.go:TestIntegrationChatForbiddenMessageAndReportRules`<br>`internal/reports/usecase/usecase_test.go:TestCreateMessageOnlyParticipants` |
| CA-07 | `internal/notifications/adapter/http/push_integration_test.go:TestIntegrationBookingRequestCreatesNoticeAndPush`<br>`internal/notifications/adapter/mongo/repo_integration_test.go:TestUnreadCountAndMarkRead`<br>`internal/notifications/usecase/usecase_test.go:TestMarkReadAll` |

## 012-integracion-entrega (8/22 con prueba)

| ID | Pruebas |
|---|---|
| RF-01 | **sin prueba** |
| RF-02 | **sin prueba** |
| RF-03 | **sin prueba** |
| RF-04 | **sin prueba** |
| RF-05 | **sin prueba** |
| RF-06 | **sin prueba** |
| RF-07 | `internal/adapter/http/middleware/ratelimit_test.go:TestRateLimitAuthPerIPWithClock` |
| RF-08 | `cmd/admin/reco_test.go:TestRecoSeed` |
| RF-09 | **sin prueba** |
| RF-10 | **sin prueba** |
| RNF-01 | **sin prueba** |
| RNF-02 | **sin prueba** |
| CA-01 | **sin prueba** |
| CA-02 | `cmd/admin/seed_integration_test.go:TestIntegrationSeedTwice`<br>`cmd/admin/seed_test.go:TestRunSubcommandSeedRefusesProduction`<br>`cmd/admin/seed_test.go:TestSeedIsIdempotent` |
| CA-03 | `cmd/admin/indexes_command_integration_test.go:TestIntegrationIndexesCommand`<br>`internal/adapter/http/readyz_integration_test.go:TestIntegrationReadyz` |
| CA-04 | `internal/adapter/http/readyz_integration_test.go:TestIntegrationReadyz` |
| CA-05 | **sin prueba** |
| CA-06 | `internal/adapter/http/middleware/ratelimit_test.go:TestRateLimitAuthPerIPWithClock` |
| CA-08 | **sin prueba** |
| CA-09 | `cmd/admin/reco_integration_test.go:TestIntegrationRecoCommands`<br>`cmd/admin/seed_test.go:TestSeedAdminWithoutRoleIsAnError`<br>`cmd/admin/seed_test.go:TestSeedDestroysPhotoWhenItemFails` |
| CA-10 | `internal/adapter/mongo/catalog/catalog_integration_test.go:TestIntegrationQueryIndexesAreUsed`<br>`internal/adapter/mongo/catalog/catalog_integration_test.go:TestQueriesCoverEveryQueryIndex`<br>`internal/adapter/mongo/catalog/purpose_test.go:TestEveryIndexHasAPurpose`<br>`internal/adapter/mongo/catalog/purpose_test.go:TestPrefixPairsAreReasoned` |
| CA-07 | **sin prueba** |

## 013-produccion (7/23 con prueba)

| ID | Pruebas |
|---|---|
| RF-01 | `scripts/trace_test.py:BenchmarkRecompute` |
| RF-02 | `scripts/trace_test.py:BenchmarkRecompute`<br>`scripts/trace_test.py:TestOther` |
| RF-03 | **sin prueba** |
| RF-04 | **sin prueba** |
| RF-05 | **sin prueba** |
| RF-06 | **sin prueba** |
| RF-07 | **sin prueba** |
| RF-08 | `cmd/admin/reco_test.go:TestRecoSeed` |
| RF-09 | **sin prueba** |
| RF-10 | **sin prueba** |
| RF-11 | **sin prueba** |
| RF-12 | **sin prueba** |
| RF-13 | **sin prueba** |
| RNF-01 | `scripts/trace_test.py:BenchmarkRecompute`<br>`scripts/trace_test.py:TestOther` |
| RNF-02 | **sin prueba** |
| CA-01 | `scripts/trace_test.py:BenchmarkRecompute`<br>`scripts/trace_test.py:TestHybrid` |
| CA-02 | `cmd/admin/seed_integration_test.go:TestIntegrationSeedTwice`<br>`cmd/admin/seed_test.go:TestRunSubcommandSeedRefusesProduction`<br>`cmd/admin/seed_test.go:TestSeedIsIdempotent` |
| CA-03 | `cmd/admin/indexes_command_integration_test.go:TestIntegrationIndexesCommand` |
| CA-04 | **sin prueba** |
| CA-05 | **sin prueba** |
| CA-06 | **sin prueba** |
| CA-07 | **sin prueba** |
| CA-08 | **sin prueba** |

## 014-ia-sugerencias (14/34 con prueba)

| ID | Pruebas |
|---|---|
| RF-03 | **sin prueba** |
| RF-08 | **sin prueba** |
| RF-19 | **sin prueba** |
| RF-17 | **sin prueba** |
| RF-01 | **sin prueba** |
| RF-02 | **sin prueba** |
| RF-04 | **sin prueba** |
| RF-05 | **sin prueba** |
| RF-06 | **sin prueba** |
| RF-07 | **sin prueba** |
| RF-09 | **sin prueba** |
| RF-10 | **sin prueba** |
| RF-11 | **sin prueba** |
| RF-12 | **sin prueba** |
| RF-13 | **sin prueba** |
| RF-14 | **sin prueba** |
| RF-15 | **sin prueba** |
| RF-16 | **sin prueba** |
| RF-18 | **sin prueba** |
| RNF-01 | **sin prueba** |
| CA-01 | `internal/suggest/integration_test.go:TestIntegrationListingSuggestions`<br>`internal/suggest/usecase/suggest_test.go:TestSuggestOwnPhoto` |
| CA-02 | `internal/suggest/integration_test.go:TestIntegrationListingSuggestions`<br>`internal/suggest/usecase/suggest_test.go:TestSuggestForeignOrEvidencePhotoNeverCallsProvider` |
| CA-03 | `internal/suggest/integration_test.go:TestIntegrationListingSuggestions`<br>`internal/suggest/usecase/suggest_test.go:TestSuggestFilteredTitleIsNotReturned` |
| CA-04 | `internal/suggest/integration_test.go:TestIntegrationListingSuggestions`<br>`internal/suggest/usecase/suggest_test.go:TestSuggestDisabledOrProviderDown` |
| CA-05 | `internal/suggest/integration_test.go:TestIntegrationListingSuggestions`<br>`internal/suggest/usecase/suggest_test.go:TestSuggestDailyLimit` |
| CA-06 | `internal/suggest/integration_test.go:TestIntegrationListingSuggestions`<br>`internal/suggest/integration_test.go:TestIntegrationNoProviderKeyInRepo` |
| CA-07 | `internal/assistant/integration_test.go:TestIntegrationProjectAssistant` |
| CA-08 | `internal/assistant/adapter/http/handler_test.go:TestMessageOKShapesTheContract`<br>`internal/assistant/adapter/search/finder_test.go:TestKeepsOrderAndEmptyMatchesAreNotNil`<br>`internal/assistant/integration_test.go:TestIntegrationProjectAssistant` |
| CA-09 | `internal/assistant/domain/prompt_test.go:TestInjectionCannotEscapeDataBlock`<br>`internal/assistant/domain/prompt_test.go:TestSystemPromptListsPOL14AndRules`<br>`internal/assistant/integration_test.go:TestIntegrationProjectAssistant` |
| CA-10 | `internal/assistant/integration_test.go:TestIntegrationProjectAssistant` |
| CA-11 | `internal/assistant/integration_test.go:TestIntegrationProjectAssistant` |
| CA-12 | `internal/assistant/integration_test.go:TestIntegrationProjectAssistant` |
| CA-13 | `internal/assistant/adapter/mongo/conversations_integration_test.go:TestIntegrationConversations`<br>`internal/assistant/integration_test.go:TestIntegrationProjectAssistant` |
| CA-14 | `internal/assistant/integration_test.go:TestIntegrationProjectAssistant` |

## 015-recomendaciones-ml (16/23 con prueba)

| ID | Pruebas |
|---|---|
| RF-01 | `internal/reco/integration_test.go:TestIntegrationHybridAndExclusionsCA01CA03` |
| RF-02 | `internal/search/adapter/mongo/search_repo_integration_test.go:TestIntegrationExclusionsCA05` |
| RF-03 | `internal/reco/domain/model/model_test.go:TestContentExact`<br>`internal/search/adapter/mongo/availability_integration_test.go:TestIntegrationAvailabilityCA03` |
| RF-04 | **sin prueba** |
| RF-05 | **sin prueba** |
| RF-06 | `internal/reco/domain/model/eval_test.go:BenchmarkRecompute`<br>`internal/reco/domain/model/eval_test.go:TestEvaluationIsReproducibleAndBeatsPopularity` |
| RF-07 | **sin prueba** |
| RF-08 | `cmd/admin/reco_test.go:TestRecoSeed` |
| RF-09 | `internal/search/adapter/mongo/search_perf_test.go:TestPerfSearch` |
| RF-10 | **sin prueba** |
| RF-11 | **sin prueba** |
| RNF-01 | `internal/reco/integration_test.go:TestIntegrationLatencyRNF01`<br>`internal/reco/integration_test.go:TestIntegrationViewsAndRateLimitCA09` |
| RNF-02 | **sin prueba** |
| RNF-03 | **sin prueba** |
| CA-01 | `internal/reco/integration_test.go:TestIntegrationHybridAndExclusionsCA01CA03`<br>`internal/reco/integration_test.go:TestMain`<br>`internal/reco/usecase/usecase_test.go:TestRecommendHybridCA01`<br>`internal/reco/usecase/usecase_test.go:TestRecordView` |
| CA-02 | `cmd/admin/seed_integration_test.go:TestIntegrationSeedTwice`<br>`cmd/admin/seed_test.go:TestRunSubcommandSeedRefusesProduction`<br>`cmd/admin/seed_test.go:TestSeedIsIdempotent`<br>`internal/reco/integration_test.go:TestIntegrationColdStartCA02` (+3) |
| CA-03 | `cmd/admin/indexes_command_integration_test.go:TestIntegrationIndexesCommand`<br>`internal/reco/integration_test.go:TestIntegrationHybridAndExclusionsCA01CA03`<br>`internal/reco/integration_test.go:TestMain`<br>`internal/reco/usecase/usecase_test.go:TestRecommendColdStartCA02` (+2) |
| CA-04 | `internal/reco/domain/model/eval_test.go:TestEvaluationIsReproducibleAndBeatsPopularity`<br>`internal/reco/integration_test.go:TestIntegrationColdStartCA02`<br>`internal/reco/integration_test.go:TestIntegrationEvaluationCA04` |
| CA-05 | `internal/reco/adapter/mongo/repos_integration_test.go:TestIntegrationRecoRepositories`<br>`internal/reco/integration_test.go:TestIntegrationDeleteHistoryCA05`<br>`internal/reco/integration_test.go:TestIntegrationEvaluationCA04`<br>`internal/reco/usecase/usecase_test.go:TestDeleteHistoryCA05` (+2) |
| CA-06 | `internal/reco/adapter/http/handler_test.go:TestDeleteHistory`<br>`internal/reco/adapter/http/handler_test.go:TestDisabledCA06`<br>`internal/reco/integration_test.go:TestIntegrationDeleteHistoryCA05`<br>`internal/reco/integration_test.go:TestIntegrationDisabledCA06` (+3) |
| CA-07 | `internal/reco/adapter/http/handler_test.go:TestDisabledCA06`<br>`internal/reco/adapter/http/handler_test.go:TestInternalErrorLogsWithoutItem`<br>`internal/reco/integration_test.go:TestIntegrationDisabledCA06`<br>`internal/reco/integration_test.go:TestIntegrationNoPersonalDataCA07` |
| CA-08 | `internal/reco/adapter/mongo/repos_integration_test.go:TestIntegrationRecoRepositories`<br>`internal/reco/integration_test.go:TestIntegrationLockCA08`<br>`internal/reco/integration_test.go:TestIntegrationNoPersonalDataCA07`<br>`internal/reco/usecase/usecase_test.go:TestConcurrentRecomputeCA08` (+1) |
| CA-09 | `cmd/admin/reco_integration_test.go:TestIntegrationRecoCommands`<br>`cmd/admin/seed_test.go:TestSeedAdminWithoutRoleIsAnError`<br>`cmd/admin/seed_test.go:TestSeedDestroysPhotoWhenItemFails`<br>`internal/reco/adapter/http/handler_test.go:TestEvent` (+5) |

