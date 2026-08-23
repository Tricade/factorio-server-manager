const defaultMaximumDimension = 2048;
const defaultDetailedTargetDimension = 1024;
const defaultMaximumPlatformScale = 64;

const mapOverlayGeometry = (surface, detailedSmallSurface = false, maximumDimension = defaultMaximumDimension) => {
    const sourceWidth = Number(surface?.width);
    const sourceHeight = Number(surface?.height);
    if (!(sourceWidth > 0) || !(sourceHeight > 0) || !(maximumDimension > 0)) return null;
    const fitScale = Math.min(maximumDimension / sourceWidth, maximumDimension / sourceHeight);
    const detailedScale = Math.max(1, defaultDetailedTargetDimension / Math.max(sourceWidth, sourceHeight));
    const outputScale = Math.min(detailedSmallSurface ? Math.min(defaultMaximumPlatformScale, detailedScale) : 1, fitScale);
    return {
        outputScale,
        width: Math.max(1, Math.round(sourceWidth * outputScale)),
        height: Math.max(1, Math.round(sourceHeight * outputScale))
    };
};

module.exports = {mapOverlayGeometry};
