import React from "react";
import brandIcon from "../../assets/brand-icon.png";

const BrandMark = ({className = "", isLoading = false}) => <span className={`ui-brand-mark${isLoading ? " ui-brand-mark--loading" : ""} ${className}`.trim()}>
    <img src={brandIcon} alt="" aria-hidden="true"/>
</span>;

export default BrandMark;
