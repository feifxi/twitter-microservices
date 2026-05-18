"use client";

import { useMutation } from "@tanstack/react-query";
import api from "@/lib/axios";
import type { PresignResult } from "@/lib/types";

// Keep in sync with backend media-service limits.
export const MAX_UPLOAD_BYTES = 10 * 1024 * 1024; // 10 MB

export class UploadTooLargeError extends Error {
	constructor(public readonly bytes: number) {
		super(`File is ${Math.round(bytes / 1024 / 1024)} MB; max is 10 MB`);
		this.name = "UploadTooLargeError";
	}
}

export function isFileTooLarge(file: File): boolean {
	return file.size > MAX_UPLOAD_BYTES;
}

// Throws on any failure so the caller can surface a toast.
async function uploadMedia(file: File): Promise<PresignResult> {
	if (file.size > MAX_UPLOAD_BYTES) {
		throw new UploadTooLargeError(file.size);
	}
	const presign = await api
		.post<PresignResult>("/v1/media/presign", { content_type: file.type })
		.then((r) => r.data);
	const res = await fetch(presign.upload_url, {
		method: "PUT",
		body: file,
		headers: { "Content-Type": file.type },
	});
	if (!res.ok) throw new Error(`Upload failed: ${res.status}`);
	return presign;
}

export function useUploadMedia() {
	return useMutation({ mutationFn: uploadMedia });
}
