import React, {useEffect, useRef} from "react";
import * as ReactDom from "react-dom";
import {FontAwesomeIcon} from "@fortawesome/react-fontawesome";
import {faExpand, faMagnifyingGlassMinus, faMagnifyingGlassPlus, faRotateLeft, faXmark} from "@fortawesome/free-solid-svg-icons";

const modalRoot = document.getElementById("modal-root");
const minimumZoom = 1;
const maximumZoom = 12;

const clampZoom = value => Math.min(maximumZoom, Math.max(minimumZoom, value));

const MapImageCanvas = ({src, alt, view, setView, onFullscreen = null, isFullscreen = false, isPixelated = false}) => {
    const viewportRef = useRef(null);
    const dragRef = useRef(null);

    const zoomAt = (factor, clientX = null, clientY = null) => {
        const viewport = viewportRef.current;
        if (!viewport) return;
        const rect = viewport.getBoundingClientRect();
        const pointX = clientX === null ? 0 : clientX - rect.left - rect.width / 2;
        const pointY = clientY === null ? 0 : clientY - rect.top - rect.height / 2;
        setView(current => {
            const zoom = clampZoom(current.zoom * factor);
            if (zoom === minimumZoom) return {zoom, x: 0, y: 0};
            const ratio = zoom / current.zoom;
            return {
                zoom,
                x: pointX - (pointX - current.x) * ratio,
                y: pointY - (pointY - current.y) * ratio
            };
        });
    };

    useEffect(() => {
        const viewport = viewportRef.current;
        if (!viewport) return undefined;
        const handleWheel = event => {
            event.preventDefault();
            zoomAt(event.deltaY < 0 ? 1.2 : 1 / 1.2, event.clientX, event.clientY);
        };
        viewport.addEventListener("wheel", handleWheel, {passive: false});
        return () => viewport.removeEventListener("wheel", handleWheel);
    });

    const startDrag = event => {
        if (event.button !== 0) return;
        event.currentTarget.setPointerCapture(event.pointerId);
        dragRef.current = {pointerId: event.pointerId, clientX: event.clientX, clientY: event.clientY, x: view.x, y: view.y};
    };

    const moveDrag = event => {
        const drag = dragRef.current;
        if (!drag || drag.pointerId !== event.pointerId) return;
        setView(current => ({
            ...current,
            x: drag.x + event.clientX - drag.clientX,
            y: drag.y + event.clientY - drag.clientY
        }));
    };

    const endDrag = event => {
        if (dragRef.current?.pointerId !== event.pointerId) return;
        dragRef.current = null;
        if (event.currentTarget.hasPointerCapture(event.pointerId)) event.currentTarget.releasePointerCapture(event.pointerId);
    };

    const reset = () => setView({zoom: minimumZoom, x: 0, y: 0});

    return <div className={`ui-map-viewer${isFullscreen ? " ui-map-viewer--fullscreen" : ""}`}>
        <div className="ui-map-viewer__controls" role="group" aria-label="Map zoom controls">
            <button type="button" onClick={() => zoomAt(1 / 1.25)} disabled={view.zoom <= minimumZoom} title="Zoom out" aria-label="Zoom out">
                <FontAwesomeIcon icon={faMagnifyingGlassMinus}/>
            </button>
            <output aria-label="Map zoom level">{Math.round(view.zoom * 100)}%</output>
            <button type="button" onClick={() => zoomAt(1.25)} disabled={view.zoom >= maximumZoom} title="Zoom in" aria-label="Zoom in">
                <FontAwesomeIcon icon={faMagnifyingGlassPlus}/>
            </button>
            <button type="button" onClick={reset} disabled={view.zoom === minimumZoom && view.x === 0 && view.y === 0} title="Fit map" aria-label="Fit map">
                <FontAwesomeIcon icon={faRotateLeft}/>
            </button>
            {onFullscreen && <button type="button" onClick={onFullscreen} title="Open fullscreen map" aria-label="Open fullscreen map">
                <FontAwesomeIcon icon={faExpand}/>
            </button>}
        </div>
        <div
            ref={viewportRef}
            className={`ui-map-viewer__canvas${dragRef.current ? " is-dragging" : ""}`}
            onPointerDown={startDrag}
            onPointerMove={moveDrag}
            onPointerUp={endDrag}
            onPointerCancel={endDrag}
            onDoubleClick={event => zoomAt(view.zoom >= 2 ? 1 / view.zoom : 2, event.clientX, event.clientY)}
            role="application"
            aria-label={`${alt}. Use the mouse wheel or zoom buttons to zoom and drag to move.`}
        >
            <img
                className={isPixelated ? "is-pixelated" : ""}
                src={src}
                alt={alt}
                draggable="false"
                style={{transform: `translate3d(${view.x}px, ${view.y}px, 0) scale(${view.zoom})`}}
            />
        </div>
    </div>;
};

export const MapImageLightbox = ({src, alt, title, isOpen, close, view, setView, isPixelated = false}) => {
    useEffect(() => {
        if (!isOpen) return undefined;
        const originalOverflow = document.body.style.overflow;
        const closeOnEscape = event => {
            if (event.key === "Escape") close();
        };
        document.body.style.overflow = "hidden";
        document.addEventListener("keydown", closeOnEscape);
        return () => {
            document.body.style.overflow = originalOverflow;
            document.removeEventListener("keydown", closeOnEscape);
        };
    }, [close, isOpen]);

    if (!isOpen) return null;
    return ReactDom.createPortal(
        <div className="ui-map-lightbox" role="dialog" aria-modal="true" aria-label={`${title} fullscreen map`} onPointerDown={event => {
            if (event.target === event.currentTarget) close();
        }}>
            <section className="ui-map-lightbox__panel">
                <header>
                    <div><span>Factory map</span><h2>{title}</h2></div>
                    <button type="button" onClick={close} title="Close fullscreen map" aria-label="Close fullscreen map">
                        <FontAwesomeIcon icon={faXmark}/>
                    </button>
                </header>
                <MapImageCanvas src={src} alt={alt} view={view} setView={setView} isFullscreen isPixelated={isPixelated}/>
            </section>
        </div>,
        modalRoot
    );
};

const MapImageViewer = props => <MapImageCanvas {...props}/>;

export default MapImageViewer;
