import { api } from "../api/client";
import { sessionRefusalWatch } from "../lib/session";
import { createQueryClient } from "./queryclient";

export const queryClient = createQueryClient();
api.use(sessionRefusalWatch(queryClient));
