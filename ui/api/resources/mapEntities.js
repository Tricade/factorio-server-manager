import parser from "./mapEntitiesParser.cjs";
import {authenticationRequiredEvent} from "../client";

const {createEntityAccumulator} = parser;

const readResponseError = async response => {
    try {
        const message = (await response.text()).trim().slice(0, 500);
        return message || `Map entity details could not be loaded (${response.status}).`;
    } catch (error) {
        return `Map entity details could not be loaded (${response.status}).`;
    }
};

const loadMapEntities = async (surfaceID, generatedAt, signal) => {
    const url = `/api/map-snapshot/surfaces/${encodeURIComponent(surfaceID)}/entities?v=${encodeURIComponent(generatedAt || "")}`;
    const response = await fetch(url, {
        credentials: "same-origin",
        headers: {Accept: "application/x-ndjson"},
        signal
    });
    if (response.status === 401) window.dispatchEvent(new Event(authenticationRequiredEvent));
    if (!response.ok) throw new Error(await readResponseError(response));

    const accumulator = createEntityAccumulator();
    if (!response.body?.getReader) {
        return accumulator.finish(await response.text());
    }

    const reader = response.body.getReader();
    const decoder = new TextDecoder("utf-8", {fatal: true});
    try {
        while (true) {
            const {done, value} = await reader.read();
            if (done) break;
            if (signal?.aborted) throw new DOMException("The request was aborted.", "AbortError");
            accumulator.push(decoder.decode(value, {stream: true}));
        }
        return accumulator.finish(decoder.decode());
    } finally {
        reader.releaseLock();
    }
};

export default loadMapEntities;
