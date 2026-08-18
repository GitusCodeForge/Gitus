package controller

import (
	"github.com/GitusCodeForge/Gitus/routes"
	"github.com/GitusCodeForge/Gitus/routes/controller/admin"
)

func InitializeRoute(context *routes.RouterContext) {
	bindDynamicAssetController(context)
	
	bindBlobController(context)
	bindBranchController(context)
	bindCommitController(context)
	bindDiffController(context)
	bindHistoryController(context)
	bindIndexController(context)
	bindRepositoryController(context)
	bindTagController(context)
	bindTreeHandler(context)
	bindAllController(context)
	bindHttpCloneController(context)
	bindShutdownNoticeController(context)
	bindMaintenanceNoticeController(context)
	bindPrivateNoticeController(context)
	bindRRDocController(context)
	
	if context.Config.UseNamespace {
		bindNamespaceController(context)
		if context.Config.IsInForgeMode() {
			bindNamespaceSettingController(context)
		}
	}
	
	if context.Config.IsInForgeMode() {
		bindUserController(context)
		bindLoginController(context)
		bindLogoutController(context)
		bindSettingController(context)
		bindSettingSSHController(context)
		bindSettingGPGController(context)
		bindSettingEmailController(context)
		bindSettingPrivacyController(context)
		bindRepositorySettingController(context)
		bindNewNamespaceController(context)
		bindNewRepositoryController(context)
		bindNewSnippetController(context)

		bindRegisterController(context)
		bindReceiptController(context)
		bindConfirmRegistrationController(context)
		bindVerifyEmailController(context)

		// bind admin controller
		admin.BindAllAdminControllers(context)

		bindResetPasswordController(context)

		bindIssueController(context)
		bindLabelController(context)

		bindSnippetController(context)
	}
}

