module.exports = {
  future: {
    removeDeprecatedGapUtilities: true,
    purgeLayersByDefault: true,
  },
  content: [
    './ui/**/*.jsx',
    './ui/**/*.html',
  ],
  theme: {
    extend: {
      width: {
        72: "18rem",
        80: "20rem",
        88: "22rem",
        96: "24rem",
      },
      margin: {
        72: "18rem",
        80: "20rem",
        88: "22rem",
        96: "24rem",
      }
    },
    colors: {
      "gray": {
        dark: "#15181D",
        medium: "#20242B",
        light: "#A8B0BC"
      },
      "white": "#F7F8FA",
      "dirty-white": "#F4E7D1",
      "green": "#35C976",
      "green-light": "#75E2A4",
      "blue": "#5B8DEF",
      "blue-light": "#8CB0F5",
      "red": "#F05D68",
      "red-light": "#F79098",
      "orange": "#F4A62A",
      "black": "#0D0F12"
    },
    boxShadow: {
      default: '0 1px 3px 0 rgba(0, 0, 0, .1), 0 1px 2px 0 rgba(0, 0, 0, .06)',
      md: '0 4px 6px -1px rgba(0, 0, 0, .1), 0 2px 4px -1px rgba(0, 0, 0, .06)',
      lg: '0 10px 15px -3px rgba(0, 0, 0, .1), 0 4px 6px -2px rgba(0, 0, 0, .05)',
      xl: '0 20px 25px -5px rgba(0, 0, 0, .1), 0 10px 10px -5px rgba(0, 0, 0, .04)',
      "2xl": '0 25px 50px -12px rgba(0, 0, 0, .25)',
      "3xl": '0 35px 60px -15px rgba(0, 0, 0, .3)',
      inner: 'inset 0 4px 8px 0 rgba(0, 0, 0, 0.9)',
      outline: '0 0 0 3px rgba(66, 153, 225, 0.5)',
      'none': 'none',
    },
    maxWidth: {
      '1/2': '50%',
    }
  },
  variants: {
    maxWidth: ['responsive']
  },
  plugins: [],
}
