import { api } from "../api/client";
import type { components } from "../api/schema";
import { ownerRequestHooks } from "./ownerrequestapi";

export type EditAccessRequest = components["schemas"]["EditAccessRequest"];

/** The record kinds a colleague may ask the owner for edit access to. */
export type EditAccessRecordType = EditAccessRequest["record_type"];

const PATH = "/records/{record_type}/{id}/edit-access-request";

const requests = ownerRequestHooks<EditAccessRecordType, EditAccessRequest>(
  "edit-access-request",
  {
    read: ({ recordType, id }) =>
      api.GET(PATH, { params: { path: { record_type: recordType, id } } }),
    send: ({ recordType, id }) =>
      api.POST(PATH, { params: { path: { record_type: recordType, id } } }),
    withdraw: ({ recordType, id }) =>
      api.DELETE(PATH, { params: { path: { record_type: recordType, id } } }),
  },
);

export const editAccessRequestKey = requests.key;

/** The viewer's own open request for edit access, or null. */
export const useEditAccessRequest = requests.useRequest;

/** Ask the record's owner for edit access, or take that request back. */
export const useEditAccessRequestActions = requests.useActions;
