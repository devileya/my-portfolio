import React from 'react';
import { StaticQuery, graphql } from 'gatsby';
import PropTypes from 'prop-types';
import { GatsbyImage, getImage } from 'gatsby-plugin-image';

const AboutImg = ({ filename, alt }) => (
  <StaticQuery
    query={graphql`
      query {
        images: allFile {
          edges {
            node {
              relativePath
              childImageSharp {
                gatsbyImageData(
                  width: 350
                  layout: FIXED
                  placeholder: BLURRED
                  transformOptions: { cropFocus: CENTER, fit: CONTAIN }
                )
              }
            }
          }
        }
      }
    `}
    render={(data) => {
      const imageNode = data.images.edges.find((n) => n.node.relativePath.includes(filename));

      if (!imageNode) return null;

      const image = getImage(imageNode.node.childImageSharp);
      return <GatsbyImage className="rounded shadow-lg" alt={alt} image={image} />;
    }}
  />
);

AboutImg.propTypes = {
  filename: PropTypes.string.isRequired,
  alt: PropTypes.string.isRequired,
};

export default AboutImg;
