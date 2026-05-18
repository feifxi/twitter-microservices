import { Toast } from "@base-ui/react/toast";

export const toastManager = Toast.createToastManager();

export const toast = {
	show: (message: string) => toastManager.add({ title: message }),
	error: (message: string) =>
		toastManager.add({ title: message, type: "error" }),
};
