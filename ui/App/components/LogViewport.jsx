import React, {useCallback, useLayoutEffect, useRef, useState} from "react";
import {FontAwesomeIcon} from "@fortawesome/react-fontawesome";
import {faArrowDown} from "@fortawesome/free-solid-svg-icons";
import Button from "./Button";

const bottomThreshold = 32;

const LogViewport = ({lines, emptyContent, ariaLive = false, label = "Log output"}) => {
    const viewportRef = useRef(null);
    const previousLinesRef = useRef([]);
    const positionedRef = useRef(false);
    const [isFollowing, setIsFollowing] = useState(true);
    const [hasNewOutput, setHasNewOutput] = useState(false);

    const scrollToLatest = useCallback(() => {
        const viewport = viewportRef.current;
        if (!viewport) return;
        viewport.scrollTop = viewport.scrollHeight;
        setIsFollowing(true);
        setHasNewOutput(false);
    }, []);

    useLayoutEffect(() => {
        const viewport = viewportRef.current;
        const previousLines = previousLinesRef.current;
        const outputChanged = previousLines.length !== lines.length
            || previousLines[0] !== lines[0]
            || previousLines[previousLines.length - 1] !== lines[lines.length - 1];

        previousLinesRef.current = lines;
        if (!viewport || lines.length === 0) {
            positionedRef.current = false;
            setHasNewOutput(false);
            return;
        }

        if (!positionedRef.current || isFollowing) {
            viewport.scrollTop = viewport.scrollHeight;
            positionedRef.current = true;
            setHasNewOutput(false);
        } else if (outputChanged) {
            setHasNewOutput(true);
        }
    }, [isFollowing, lines]);

    const handleScroll = useCallback(event => {
        const viewport = event.currentTarget;
        const atBottom = viewport.scrollHeight - viewport.scrollTop - viewport.clientHeight <= bottomThreshold;
        setIsFollowing(atBottom);
        if (atBottom) setHasNewOutput(false);
    }, []);

    return <div className="ui-log-frame">
        <div
            ref={viewportRef}
            className="ui-terminal p-4"
            onScroll={handleScroll}
            role={ariaLive ? "log" : "region"}
            aria-label={label}
            aria-live={ariaLive ? "polite" : undefined}
            tabIndex={0}
        >
            {lines.length === 0 ? emptyContent : lines.map((line, index) => (
                <div className="whitespace-pre-wrap break-all" key={`${index}-${line}`}>{line}</div>
            ))}
        </div>
        {!isFollowing && <Button
            type="secondary"
            size="sm"
            className="ui-log-jump"
            onClick={scrollToLatest}
        >
            <FontAwesomeIcon icon={faArrowDown}/>
            {hasNewOutput ? " New output · Jump to latest" : " Jump to latest"}
        </Button>}
    </div>;
};

export default LogViewport;
